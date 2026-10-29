//go:build windows

package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHelperWithoutConsole(t *testing.T) {
	if os.Getenv("ONESUB_TEST_NO_CONSOLE") == "1" {
		h, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
		fmt.Printf("console=%d", h)
		os.Exit(0)
	}
	cmd := CommandContext(context.Background(), os.Args[0], "-test.run=^TestHelperWithoutConsole$")
	cmd.Env = append(os.Environ(), "ONESUB_TEST_NO_CONSOLE=1")
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("visible startup configuration")
	}
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) != "console=0" {
		t.Fatal(string(output), err)
	}
}

// Tools report their stages on stderr: each line arrives whole, however
// the child splits its writes, and the last one without a newline too.
func TestRunLinesPassesStderrLines(t *testing.T) {
	if os.Getenv("ONESUB_TEST_STDERR") == "1" {
		os.Stderr.WriteString("[stage] one\r\n[sta")
		os.Stderr.WriteString("ge] two 1/3\nlast")
		os.Exit(0)
	}
	cmd := CommandContext(context.Background(), os.Args[0], "-test.run=^TestRunLinesPassesStderrLines$")
	cmd.Env = append(os.Environ(), "ONESUB_TEST_STDERR=1")
	var lines []string
	if err := RunLines(context.Background(), cmd, nil, func(line string) { lines = append(lines, line) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(lines, "|"); got != "[stage] one|[stage] two 1/3|last" {
		t.Fatalf("stderr lines = %q", got)
	}
}
