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
		a.playerSurface(c)
		a.transportBar(c)
	})
}

// playerSurface keeps the same WebView mounted throughout import and tasks.
// Native letterboxing keeps its viewport at the media aspect, so WebView2's
// asynchronous resize cannot stretch an old wide page into a tall rectangle.
func (a *NativeApp) playerSurface(c *ui.Context) {
	area := ui.Column(c).Grow(1).MinHeight(0).Center().Clip().Background(ui.RGB(0, 0, 0)).Label(locales.A11yPlayerSurface())
	area.Children(func() {
		if a.player != nil {
			room := area.Bounds()
			last := ui.Local(area, "viewport-room", func() ui.Rect { return ui.Rect{} })
			if *last != room {
				*last = room
				c.After(0) // settle a changed native viewport before the next player poll
			}
			w, h := fitVideoViewport(room.W, room.H, a.frameAspect)
			ui.WebView(c.Key("player-webview"), a.player).Size(w, h).Shrink(0).AlignSelf(ui.Center).Label(locales.A11yPlayerViewport())
		}
		// Controls and task progress overlay the fixed media area; they do
		// not become children that contribute intrinsic height to the WebView.
		ui.Column(c.Key("player-overlay")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
			switch {
			case a.recognitionRunning:
				ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { a.recognitionProgressView(c) })
			case a.composeRunning:
				ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { a.composeProgressView(c) })
			default:
				a.playerOverlay(c)
			}
		})
	})
}

// Contain without changing the source's display aspect (including SAR).
func fitVideoViewport(width, height float32, aspect float64) (float32, float32) {
	if aspect <= 0 || math.IsNaN(aspect) || math.IsInf(aspect, 0) {
		aspect = 16.0 / 9
	}
	w := min(max(0, width), max(0, height)*float32(aspect))
	return w, w / float32(aspect)
}

// transportBar: the position across the whole picture, then the transport
// buttons and the speed, the time, and at the end the volume, as video
// players and editors' monitors lay them out. A narrow player keeps every
// control whole: transport, time and volume wrap as complete groups when
// needed, instead of squeezing their labels. Times use tabular digits.
func (a *NativeApp) transportBar(c *ui.Context) {
	// The picture gives way to a task's progress, and so does its transport.
	disabled := !a.playbackAvailable()
	ui.Column(c).Padding(em(c, 2), em(c, 12), em(c, 6), em(c, 12)).Shrink(0).Label(locales.A11yPlayerControls()).Children(func() {
		value := a.position
		ui.Slider(c, &value, 0, math.Max(a.duration, .001)).Label(locales.PlayerPosition()).Disabled(disabled).OnChange(func() { a.evalPlayer(fmt.Sprintf("video.currentTime = %f", value)) })
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
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
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Toolbar(c, func() {
					toolIcon(c, muteLabel, muteIcon).OnClick(func() { a.toggleMute(c.Now()) })
				}).Padding(0)
				volume := a.audibleVolume()
				ui.Slider(c, &volume, 0, 100).Width(em(c, 96)).Label(locales.PlayerVolume()).Tooltip(locales.PlayerVolumeValue(int(math.Round(volume)))).OnChange(func() {
					// Moving the volume is a request to hear it.
					a.setVolume(volume, false, c.Now())
				})
			})
		})
	})
}

// playerOverlay is a transparent native layer over the page. It takes the
// pointer there, so clicks toggle playback and the context menu is MyGo's,
// in the app's language, never WebView2's media menu.
func (a *NativeApp) playerOverlay(c *ui.Context) {
	// The empty player must own pointer/drop hit testing too. Otherwise
	// WebView2 receives the first file drop as a browser navigation instead
	// of the application's native import service (subsequent drops differ).
	layer := ui.Column(c).Fill().Center().OnClick(a.togglePlayback)
	if a.mediaURL == "" {
		layer.Children(func() {
			iconButton(c, locales.PlayerAddVideo(), "folder-open").Disabled(a.busy).OnClick(a.pickFiles)
		})
		return
	}
	layer.Label(locales.A11yVideo()).ContextMenu(func(m *ui.Menu) {
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

func (a *NativeApp) playbackAvailable() bool {
	return a.mediaURL != "" && !a.composeRunning && !a.recognitionRunning
}

func (a *NativeApp) togglePlayback() {
	if a.playbackAvailable() {
		a.evalPlayer("video.paused ? video.play() : video.pause()")
	}
}

// Native shortcut routing leaves spaces to text inputs and focused controls,
// and suppresses workbench shortcuts behind a dialog.
func (a *NativeApp) playbackShortcut(c *ui.Context) bool {
	return a.playbackAvailable() && c.Shortcut(0, ui.KeySpace)
}
func (a *NativeApp) seekBy(seconds float64) {
	if !a.playbackAvailable() {
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

// Keep the saved level while muted, but show and send the audible level as
// zero. Unmuting restores the saved value; dragging the slider unmutes it.
func (a *NativeApp) audibleVolume() float64 {
	if a.settings.Muted {
		return 0
	}
	return a.settings.Volume
}

func (a *NativeApp) volumeScript() string {
	return fmt.Sprintf("video.volume=%.3f;video.muted=%t;", a.audibleVolume()/100, a.settings.Muted)
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
