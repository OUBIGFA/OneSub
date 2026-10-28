package main

import (
	"fmt"
	"math"
	"path/filepath"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func playerButton(c *ui.Context, label, icon string) ui.Element {
	// Keep MyGo's button and icon sizing in the same font-scaled box. Its
	// normal text-button padding leaves too little room in a fixed 36px box.
	return ui.Button(c, "").Label(label).Tooltip(label).Padding(c.Theme().Space(2)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons[icon]).Label(locales.A11yIconOf(label))
	})
}

func (a *NativeApp) playerView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12).Gap(8).Label(locales.A11yPlayer()).Children(func() {
		// Equal flexible cells on both sides keep the file name centred
		// whatever the width of the action on the left.
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).Children(func() {
				iconButton(c, locales.ActionRecognize(), "audio-lines").Disabled(a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "").OnClick(a.recognize)
			})
			name := ""
			if a.snapshot != nil && a.snapshot.Manifest.MediaPath != "" {
				name = filepath.Base(a.snapshot.Manifest.MediaPath)
			}
			ui.Text(c, name).TextColor(c.Theme().TextMuted).SingleLine().Shrink(1).MinWidth(0).Tooltip(name)
			ui.Row(c).Grow(1).Basis(0)
		})
		switch {
		case a.composeRunning:
			a.composeProgressView(c)
		case a.player != nil:
			// The page is always placed, black and laid out at the player's
			// size, so a first video only changes its source.
			ui.WebView(c, a.player).Grow(1).Children(func() { a.playerOverlay(c) })
		default:
			ui.Column(c).Grow(1).Center().Children(func() {
				iconButton(c, locales.PlayerAddVideo(), "folder-open").Disabled(a.busy).OnClick(a.pickFiles)
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label, icon := locales.PlayerPlay(), "play"
			if !a.paused {
				label, icon = locales.PlayerPause(), "pause"
			}
			disabled := a.mediaURL == "" || a.composeRunning
			playerButton(c, locales.PlayerBack(), "rewind").Disabled(disabled).OnClick(func() { a.seekBy(-15) })
			playerButton(c, label, icon).Disabled(disabled).OnClick(a.togglePlayback)
			playerButton(c, locales.PlayerForward(), "fast-forward").Disabled(disabled).OnClick(func() { a.seekBy(15) })
			ui.Text(c, stamp(a.position)).SingleLine().MarginStart(8)
			value := a.position
			upper := math.Max(a.duration, .001)
			ui.Slider(c, &value, 0, upper).Label(locales.PlayerPosition()).Disabled(disabled).Grow(1).OnChange(func() { a.evalPlayer(fmt.Sprintf("video.currentTime = %f", value)) })
			ui.Text(c, stamp(a.duration)).SingleLine()
			muteLabel, muteIcon := locales.PlayerMute(), "volume-2"
			if a.settings.Muted || a.settings.Volume == 0 {
				muteLabel, muteIcon = locales.PlayerUnmute(), "volume-x"
			}
			playerButton(c, muteLabel, muteIcon).MarginStart(8).OnClick(func() { a.toggleMute(c.Now()) })
			volume := a.settings.Volume
			ui.Slider(c, &volume, 0, 100).Width(110).Label(locales.PlayerVolume()).Tooltip(locales.PlayerVolumeValue(int(math.Round(volume)))).OnChange(func() {
				// Moving the volume is a request to hear it.
				a.setVolume(volume, false, c.Now())
			})
		}).Label(locales.A11yPlayerControls())
	})
}

// playerOverlay is a transparent native layer over the page. It takes the
// pointer there, so clicks toggle playback and the context menu is MyGo's,
// in the app's language, never WebView2's media menu.
func (a *NativeApp) playerOverlay(c *ui.Context) {
	layer := ui.Column(c).Fill().Center()
	if a.mediaURL == "" {
		layer.Children(func() {
			iconButton(c, locales.PlayerAddVideo(), "folder-open").Disabled(a.busy).OnClick(a.pickFiles)
		})
		return
	}
	layer.Label(locales.A11yVideo()).OnClick(a.togglePlayback).ContextMenu(func(m *ui.Menu) {
		disabled := a.composeRunning
		playLabel := locales.PlayerPlay()
		if !a.paused {
			playLabel = locales.PlayerPause()
		}
		if m.Item(playLabel).Disabled(disabled).Chosen() {
			a.togglePlayback()
		}
		if m.Item(locales.PlayerBack()).Disabled(disabled).Chosen() {
			a.seekBy(-15)
		}
		if m.Item(locales.PlayerForward()).Disabled(disabled).Chosen() {
			a.seekBy(15)
		}
		m.Separator()
		muteLabel := locales.PlayerMute()
		if a.settings.Muted || a.settings.Volume == 0 {
			muteLabel = locales.PlayerUnmute()
		}
		if m.Item(muteLabel).Chosen() {
			a.toggleMute(time.Now())
		}
	})
}

func (a *NativeApp) togglePlayback() {
	if a.mediaURL != "" && !a.composeRunning {
		a.evalPlayer("video.paused ? video.play() : video.pause()")
	}
}
func (a *NativeApp) seekBy(seconds float64) {
	if a.mediaURL == "" || a.composeRunning {
		return
	}
	if seconds < 0 {
		a.evalPlayer(fmt.Sprintf("video.currentTime=Math.max(0,video.currentTime-%g)", -seconds))
		return
	}
	a.evalPlayer(fmt.Sprintf("video.currentTime=Math.min(Number.isFinite(video.duration)?video.duration:video.currentTime+%g,video.currentTime+%g)", seconds, seconds))
}

// setVolume applies the level at once and remembers it shortly after the
// slider stops, as other preferences are.
func (a *NativeApp) setVolume(percent float64, muted bool, now time.Time) {
	a.settings.Volume = max(0, min(100, percent))
	a.settings.Muted = muted
	a.evalPlayer(a.volumeScript())
	a.prefsSaveDue = now.Add(400 * time.Millisecond)
}

// toggleMute restores a silent slider to full volume, else flips mute.
func (a *NativeApp) toggleMute(now time.Time) {
	if a.settings.Volume == 0 {
		a.setVolume(100, false, now)
		return
	}
	a.setVolume(a.settings.Volume, !a.settings.Muted, now)
}
func (a *NativeApp) volumeScript() string {
	return fmt.Sprintf("video.volume=%.3f;video.muted=%t;", a.settings.Volume/100, a.settings.Muted)
}

// playerErrorText says the page's error codes in the app's language.
func playerErrorText(code string) string {
	switch code {
	case "unsupported-video":
		return locales.ErrVideoUnsupported()
	case "subtitle-preview":
		return locales.ErrSubtitlePreview()
	}
	return code
}
