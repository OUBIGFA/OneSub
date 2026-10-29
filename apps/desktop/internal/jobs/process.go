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
	n := len(p)
	b.data = append(b.data, p...)
	if len(b.data) > 32768 {
		b.data = append([]byte{}, b.data[len(b.data)-32768:]...)
	}
	return n, nil
}

// stderrLines keeps the bounded tail for error reports and passes each
// complete line on as the child writes it, as tools report their stages
// there. A line longer than 64 KiB is cut to its last 64 KiB.
type stderrLines struct {
	tailBuffer
	line    func(string)
	partial []byte
}

func (s *stderrLines) Write(p []byte) (int, error) {
	s.tailBuffer.Write(p)
	s.partial = append(s.partial, p...)
	for {
		i := bytes.IndexByte(s.partial, '\n')
		if i < 0 {
			break
		}
		s.line(strings.TrimRight(string(s.partial[:i]), "\r"))
		s.partial = s.partial[i+1:]
	}
	if len(s.partial) > 65536 {
		s.partial = append([]byte{}, s.partial[len(s.partial)-65536:]...)
	}
	return len(p), nil
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
