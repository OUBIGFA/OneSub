package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// playerView is the program monitor: a header with the transcription action
// and the file name, the picture edge to edge on black, and the transport.
func (a *NativeApp) playerView(c *ui.Context) {
	ui.Column(c).Fill().Label(locales.A11yPlayer()).Children(func() {
		name := ""
		if a.snapshot != nil && a.snapshot.Manifest.MediaPath != "" {
			name = filepath.Base(a.snapshot.Manifest.MediaPath)
		}
		panelHeader(c, nil, func() {
			muted(c, name).SingleLine().Shrink(1).MinWidth(0).Tooltip(name)
		}, nil)
		switch {
		case a.recognitionRunning:
			a.recognitionProgressView(c)
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
		a.transportBar(c)
	})
}

// transportBar: the position across the whole picture, then the transport
// buttons and the speed, the time, and at the end the volume, as video
// players and editors' monitors lay them out. A narrow player keeps every
// control whole: only the volume slider gives up room. Times use digits of
// one width so they do not shift as they count.
func (a *NativeApp) transportBar(c *ui.Context) {
	// The picture gives way to a task's progress, and so does its transport.
	disabled := a.mediaURL == "" || a.composeRunning || a.recognitionRunning
	ui.Column(c).Padding(em(c, 2), em(c, 12), em(c, 6), em(c, 12)).Shrink(0).Label(locales.A11yPlayerControls()).Children(func() {
		value := a.position
		ui.Slider(c, &value, 0, math.Max(a.duration, .001)).Label(locales.PlayerPosition()).Disabled(disabled).OnChange(func() { a.evalPlayer(fmt.Sprintf("video.currentTime = %f", value)) })
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			ui.Toolbar(c, func() {
				toolIcon(c, locales.PlayerBack(), "rewind").Disabled(disabled).OnClick(func() { a.seekBy(-15) })
				label, name := locales.PlayerPlay(), "play"
				if !a.paused {
					label, name = locales.PlayerPause(), "pause"
				}
				toolIcon(c, label, name).Tooltip(locales.PlayerSpaceShortcut()).Disabled(disabled).OnClick(a.togglePlayback)
				toolIcon(c, locales.PlayerForward(), "fast-forward").Disabled(disabled).OnClick(func() { a.seekBy(15) })
				ui.MenuButton(c, rateLabel(a.playbackRate()), func(m *ui.Menu) {
					for _, rate := range playbackRates {
						if m.Item(rateLabel(rate)).Checked(rate == a.playbackRate()).Chosen() {
							a.setPlaybackRate(rate)
						}
					}
				}).Label(locales.PlayerSpeed()).Tooltip(locales.PlayerSpeed()).Disabled(disabled).FontFeatures("tnum")
				// Loop playback, off until chosen, for this session.
				if ui.Toggle(c, &a.loop, "").Label(locales.PlayerLoop()).Tooltip(locales.PlayerLoop()).Padding(c.Theme().Space(2)).Disabled(disabled).Children(func() { icon(c, "repeat") }).Changed() {
					a.evalPlayer(a.rateScript())
				}
			}).Gap(em(c, 2)).Shrink(0)
			ui.Row(c).Gap(em(c, 6)).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, stamp(a.position)).FontFeatures("tnum").SingleLine()
				muted(c, "/").SingleLine()
				muted(c, stamp(a.duration)).FontFeatures("tnum").SingleLine()
			})
			ui.Spacer(c)
			muteLabel, muteIcon := locales.PlayerMute(), "volume-2"
			if a.settings.Muted || a.settings.Volume == 0 {
				muteLabel, muteIcon = locales.PlayerUnmute(), "volume-x"
			}
			ui.Toolbar(c, func() {
				toolIcon(c, muteLabel, muteIcon).OnClick(func() { a.toggleMute(c.Now()) })
			}).Shrink(0)
			volume := a.settings.Volume
			ui.Slider(c, &volume, 0, 100).Width(em(c, 96)).MinWidth(em(c, 48)).Shrink(1).Label(locales.PlayerVolume()).Tooltip(locales.PlayerVolumeValue(int(math.Round(volume)))).OnChange(func() {
				// Moving the volume is a request to hear it.
				a.setVolume(volume, false, c.Now())
			})
		})
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

// Native shortcut routing leaves spaces to text inputs and focused controls,
// and suppresses workbench shortcuts behind a dialog.
func (a *NativeApp) playbackShortcut(c *ui.Context) bool {
	return a.mediaURL != "" && !a.composeRunning && !a.recognitionRunning && c.Shortcut(0, ui.KeySpace)
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

// toggleMute restores a silent slider to the default volume, else flips mute.
func (a *NativeApp) toggleMute(now time.Time) {
	if a.settings.Volume == 0 {
		a.setVolume(settings.Default().Volume, false, now)
		return
	}
	a.setVolume(a.settings.Volume, !a.settings.Muted, now)
}
func (a *NativeApp) volumeScript() string {
	return fmt.Sprintf("video.volume=%.3f;video.muted=%t;", a.settings.Volume/100, a.settings.Muted)
}

// playbackRates are the speeds of the speed menu, slow to fast. The page's
// video element cannot play backwards (Chromium rejects negative rates).
var playbackRates = []float64{0.5, 0.75, 1, 1.25, 1.5, 2}

// playbackRate is the speed chosen for this session, normal until one is.
func (a *NativeApp) playbackRate() float64 {
	if a.rate == 0 {
		return 1
	}
	return a.rate
}

// setPlaybackRate plays at rate, also the next videos: a new source starts
// at the default rate, which is set with it.
func (a *NativeApp) setPlaybackRate(rate float64) {
	a.rate = rate
	a.evalPlayer(a.rateScript())
}
func (a *NativeApp) rateScript() string {
	return fmt.Sprintf("video.defaultPlaybackRate=%g;video.playbackRate=%g;video.loop=%t;", a.playbackRate(), a.playbackRate(), a.loop)
}

// rateLabel writes a speed as players do: 0.75×, 1×, 2×.
func rateLabel(rate float64) string {
	return strconv.FormatFloat(rate, 'f', -1, 64) + "×"
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
