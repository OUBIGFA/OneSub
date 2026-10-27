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
