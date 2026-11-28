// Package jobs owns bounded child processes and their cancellation lifetime.
package jobs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// CommandContext is the shared entry for one-shot helpers that exchange binary
// or JSON output. Windows workers must not allocate or flash a console window.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	configure(cmd)
	return cmd
}

type tailBuffer struct{ data []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = appendTail(b.data, p, 32768)
	return len(p), nil
}

// appendTail reuses storage and discards excess bytes before appending. A busy
// child must not allocate a new log buffer for every write or one huge line.
func appendTail(dst, p []byte, limit int) []byte {
	if len(p) >= limit {
		return append(dst[:0], p[len(p)-limit:]...)
	}
	if discard := len(dst) + len(p) - limit; discard > 0 {
		dst = dst[:copy(dst, dst[discard:])]
	}
	return append(dst, p...)
}

// stderrLines keeps the bounded tail for error reports and passes each
// complete line on as the child writes it, as tools report their stages
// there. A line longer than 64 KiB is cut to its last 64 KiB.
type stderrLines struct {
	tailBuffer
	line    func(string)
	partial []byte
	skipLF  bool
}

func (s *stderrLines) Write(p []byte) (int, error) {
	s.tailBuffer.Write(p)
	n := len(p)
	for len(p) > 0 {
		if s.skipLF {
			s.skipLF = false
			if p[0] == '\n' {
				p = p[1:]
				continue
			}
		}
		i := bytes.IndexAny(p, "\r\n")
		if i < 0 {
			s.partial = appendTail(s.partial, p, 65536)
			break
		}
		s.partial = appendTail(s.partial, p[:i], 65536)
		s.line(string(s.partial))
		s.partial = s.partial[:0]
		s.skipLF = p[i] == '\r'
		p = p[i+1:]
	}
	return n, nil
}

// Run consumes stdout concurrently and retains only a bounded stderr tail. On
// Windows cancellation terminates this job object, never processes by name.
func Run(ctx context.Context, cmd *exec.Cmd, line func(string)) error {
	return RunLines(ctx, cmd, line, nil)
}

// RunLines is Run that also passes each line of stderr to errLine, when set.
func RunLines(ctx context.Context, cmd *exec.Cmd, line, errLine func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	group, err := newProcessGroup()
	if err != nil {
		return err
	}
	defer group.close()
	configure(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	if errLine != nil {
		lines := &stderrLines{line: errLine}
		stderr, cmd.Stderr = &lines.tailBuffer, lines
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = group.attach(cmd.Process); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return apperr.Wrap(locales.ErrJobControl(err.Error()), err)
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	defer func() { close(done); <-stopped }()
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			group.terminate()
		case <-done:
		}
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if line != nil {
			line(scanner.Text())
		}
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		group.terminate()
	}
	err = cmd.Wait()
	if lines, ok := cmd.Stderr.(*stderrLines); ok && len(lines.partial) > 0 {
		// The last line may end without a newline.
		errLine(strings.TrimRight(string(lines.partial), "\r"))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scanErr != nil {
		return apperr.Wrap(locales.ErrJobMessage(scanErr.Error()), scanErr)
	}
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(stderr.data)))
	}
	return nil
}
