package main

import (
	"os"
	"runtime"
	"slices"
	"strings"
)

// WebView2's separate video plane can retain a stale transform on the first
// paused frame, while the page and ASS already use the final viewport. Use
// Chromium's GPU-driver workaround, not the obsolete hyphenated browser switch.
// This keeps GPU decoding and MyGo's required DirectComposition host enabled.
// The native pixel regression must run as OneSub.exe: renamed probes can mask
// the affected runtime path. See tests/e2e/player_resize.py.
func configurePlayerRuntime() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	const key = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	const flag = "--disable_direct_composition_video_overlays"
	arguments := os.Getenv(key)
	if slices.Contains(strings.Fields(arguments), flag) {
		return nil
	}
	return os.Setenv(key, strings.TrimSpace(arguments+" "+flag))
}
