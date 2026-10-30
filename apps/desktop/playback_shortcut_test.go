package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSpaceRoutesToPlaybackWithoutTakingTextOrDialogKeys(t *testing.T) {
	a := nativeForTest(t)
	a.mediaURL = "local-test-video"
	text := "subtitle"
	open := false
	presses := 0
	tt := settled(func(c *ui.Context) {
		ui.TextArea(c, &text).Label("Editor").Width(200).Height(80)
		ui.Modal(c, &open, func() { ui.Text(c, "Dialog") })
		if a.playbackShortcut(c) {
			presses++
		}
	}, 500, 400)
	tt.Key(0, ui.KeySpace)
	if presses != 1 {
		t.Fatal("Space did not request playback", presses)
	}
	if err := tt.Click("Editor"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.TypeKey(0, ui.KeySpace, " ")
	if text != " " || presses != 1 {
		t.Fatal("Space interfered with text entry", text, presses)
	}
	tt.ClickAt(400, 350)
	open = true
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	if presses != 1 {
		t.Fatal("playback changed behind a dialog")
	}
	open = false
	for _, state := range []string{"no-media", "recognizing", "composing"} {
		a.mediaURL = "local-test-video"
		a.recognitionRunning, a.composeRunning = false, false
		switch state {
		case "no-media":
			a.mediaURL = ""
		case "recognizing":
			a.recognitionRunning = true
		case "composing":
			a.composeRunning = true
		}
		tt.Frame()
		tt.Key(0, ui.KeySpace)
		if presses != 1 {
			t.Fatal("Space bypassed unavailable playback", state)
		}
	}
}
