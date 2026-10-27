// Package jobs owns bounded child processes and their cancellation lifetime.
package jobs

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
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

// Run consumes stdout concurrently and retains only a bounded stderr tail. On
// Windows cancellation terminates this job object, never processes by name.
func Run(ctx context.Context, cmd *exec.Cmd, line func(string)) error {
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
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = group.attach(cmd.Process); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return fmt.Errorf("无法管理任务进程：%w", err)
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scanErr != nil {
		return fmt.Errorf("任务消息无效：%w", scanErr)
	}
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(stderr.data)))
	}
	return nil
}
