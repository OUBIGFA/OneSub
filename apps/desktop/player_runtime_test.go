package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestPlayerRuntimeKeepsGPUAndExistingArguments(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("WebView2 compatibility is Windows-only")
	}
	const key = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	const flag = "--disable_direct_composition_video_overlays"
	for _, original := range []string{"", `--user-agent="Local player" --lang=en`, flag, "--lang=en " + flag} {
		t.Run(original, func(t *testing.T) {
			t.Setenv(key, original)
			if err := configurePlayerRuntime(); err != nil {
				t.Fatal(err)
			}
			want := original
			if !strings.Contains(want, flag) {
				want = strings.TrimSpace(want + " " + flag)
			}
			if got := os.Getenv(key); got != want {
				t.Fatal("changed user arguments or disabled more than video overlays", got, want)
			}
			if err := configurePlayerRuntime(); err != nil || os.Getenv(key) != want {
				t.Fatal("reconfiguration duplicated the workaround", err)
			}
		})
	}
}
