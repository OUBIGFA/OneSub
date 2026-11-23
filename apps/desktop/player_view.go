package main

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The global file bar names the document. Give the entire monitor above
// the transport to the picture, including before a file has been opened.
func (a *NativeApp) playerView(c *ui.Context) {
	width, _ := c.Size()
	a.playerViewAtWidth(c, width)
}

func (a *NativeApp) playerViewAtWidth(c *ui.Context, width float32) {
	ui.Column(c).Fill().Label(locales.A11yPlayer()).Children(func() {
		a.playerSurface(c)
		a.transportBar(c, width)
	})
}

// playerSurface keeps the same WebView mounted throughout import and tasks.
// Native letterboxing keeps its viewport at the media aspect, so WebView2's
// asynchronous resize cannot stretch an old wide page into a tall rectangle.
func (a *NativeApp) playerSurface(c *ui.Context) {
	area := ui.Column(c).Grow(1).MinHeight(0).Center().Clip().Background(ui.RGB(0, 0, 0)).Label(locales.A11yPlayerSurface())
	area.Children(func() {
		if a.player != nil {
			aspect := a.frameAspect
			if !(aspect > 0) || math.IsInf(aspect, 0) {
				aspect = 16.0 / 9
			}
			// Native containment resolves both the WebView placement and its
			// hole from this frame, not the previous frame's Bounds().
			ui.WebView(c.Key("player-webview"), a.player).Fill().MinHeight(0).AspectRatio(float32(aspect)).Fit(ui.Contain).Label(locales.A11yPlayerViewport())
		}
		// Controls and task progress overlay the fixed media area; they do
		// not become children that contribute intrinsic height to the WebView.
		ui.Column(c.Key("player-overlay")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
			switch {
			case a.recognitionRunning:
				ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { a.recognitionProgressView(c) })
			case a.composeRunning && !a.composeInBackground:
				ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { a.composeProgressView(c) })
			default:
				a.playerOverlay(c)
			}
		})
	})
}

// Equal grid tracks center transport on the picture, independently of the
// side controls. Narrow panes put those controls below the centered transport.
func (a *NativeApp) transportBar(c *ui.Context, width float32) {
	disabled := !a.playbackAvailable()
	ui.Column(c).Padding(em(c, 4), em(c, 12), em(c, 8), em(c, 12)).Gap(em(c, 4)).Shrink(0).Background(studioColors(c).panel).Label(locales.A11yPlayerControls()).Children(func() {
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, stamp(a.position)).FontSize(em(c, captionSize)).FontFeatures("tnum").NoWrap().Shrink(0)
			value := a.position
			lobeSlider(c, &value, 0, math.Max(a.duration, .001)).Grow(1).MinWidth(0).Label(locales.PlayerPosition()).Disabled(disabled).OnChange(func() { a.evalPlayer(fmt.Sprintf("video.currentTime = %f", value)) })
			ui.Text(c, stamp(a.duration)).FontSize(em(c, captionSize)).FontFeatures("tnum").NoWrap().Shrink(0)
		})
		compact := width < em(c, 640)
		commands := ui.Grid(c).ColumnTracks(ui.Fr(1), ui.FitContent(), ui.Fr(1)).Gap(em(c, 8)).AlignItems(ui.Center)
		if compact {
			commands.ColumnTracks(ui.FitContent(), ui.Fr(1))
		}
		commands.Children(func() {
			transport := ui.Toolbar(c, func() {
				toolIcon(c, locales.PlayerStart(), "skip-back").Disabled(disabled).OnClick(a.seekStart)
				toolIcon(c, locales.PlayerBack(), "rewind").Disabled(disabled).OnClick(func() { a.seekBy(-15) })
				toolIcon(c, locales.PlayerPreviousCue(), "chevron-left").Disabled(a.adjacentPlayerCue(-1) < 0).OnClick(func() { a.stepPlayerCue(-1) })
				label, name := locales.PlayerPlay(), "play"
				if !a.paused {
					label, name = locales.PlayerPause(), "pause"
				}
				play := toolIcon(c, label, name).Size(em(c, 42), em(c, 42)).Radius(em(c, 21)).Tooltip(locales.PlayerSpaceShortcut()).Disabled(disabled).OnClick(a.togglePlayback)
				if !disabled {
					play.Background(c.Theme().Text).TextColor(c.Theme().Background)
				}
				toolIcon(c, locales.PlayerNextCue(), "chevron-right").Disabled(a.adjacentPlayerCue(1) < 0).OnClick(func() { a.stepPlayerCue(1) })
				toolIcon(c, locales.PlayerForward(), "fast-forward").Disabled(disabled).OnClick(func() { a.seekBy(15) })
				toolIcon(c, locales.PlayerEnd(), "skip-forward").Disabled(disabled || a.duration <= 0).OnClick(a.seekEnd)
			}).Gap(0).Padding(0).AlignItems(ui.Center).JustifySelf(ui.Center).ColumnStart(2).RowStart(1)
			sideRow, audioColumn := 1, 3
			if compact {
				transport.ColumnStart(1).ColumnSpan(2)
				sideRow, audioColumn = 2, 2
			}
			ui.Toolbar(c, func() {
				rates := make([]menuCommand, len(playbackRates))
				for i, rate := range playbackRates {
					rates[i] = menuCommand{Label: rateLabel(rate), Checkable: true, Checked: rate == a.playbackRate(), Run: func() { a.setPlaybackRate(rate) }}
				}
				lobeMenuButton(c, rateLabel(a.playbackRate()), "", false, func() []menuCommand { return rates }).Label(locales.PlayerSpeed()).Tooltip(locales.PlayerSpeed()).Disabled(disabled).FontFeatures("tnum")
				if lobeToggle(c, &a.loop, "").Label(locales.PlayerLoop()).Tooltip(locales.PlayerLoop()).Padding(c.Theme().Space(2)).Disabled(disabled).Children(func() { icon(c, "repeat") }).Changed() {
					a.evalPlayer(a.rateScript())
				}
			}).Padding(0).Gap(em(c, 4)).ColumnStart(1).RowStart(sideRow).JustifySelf(ui.Start)
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).ColumnStart(audioColumn).RowStart(sideRow).JustifySelf(ui.End).Children(func() {
				muteLabel, muteIcon := locales.PlayerMute(), "volume-2"
				if a.settings.Muted || a.settings.Volume == 0 {
					muteLabel, muteIcon = locales.PlayerUnmute(), "volume-x"
				}
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Children(func() {
					ui.Toolbar(c, func() { toolIcon(c, muteLabel, muteIcon).OnClick(func() { a.toggleMute(c.Now()) }) }).Padding(0)
					volume := a.audibleVolume()
					lobeSlider(c, &volume, 0, 100).Width(em(c, 88)).Label(locales.PlayerVolume()).Tooltip(locales.PlayerVolumeValue(int(math.Round(volume)))).OnChange(func() { a.setVolume(volume, false, c.Now()) })
				})
				label, glyph := locales.PlayerFullscreen(), "maximize"
				if a.playerFullscreen {
					label, glyph = locales.PlayerExitFullscreen(), "minimize"
				}
				toolIcon(c, label, glyph).Tooltip(label + " · F11").Disabled(!a.playerFullscreen && disabled).OnClick(func() { a.setPlayerFullscreen(!a.playerFullscreen) })
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
	layer.Label(locales.A11yVideo())
	disabled := !a.playbackAvailable()
	playLabel := locales.PlayerPlay()
	if !a.paused {
		playLabel = locales.PlayerPause()
	}
	muteLabel := locales.PlayerMute()
	if a.settings.Muted || a.settings.Volume == 0 {
		muteLabel = locales.PlayerUnmute()
	}
	lobeContextMenu(c, layer, func() []menuCommand {
		return []menuCommand{
			{Label: playLabel, Disabled: disabled, Run: a.togglePlayback},
			{Label: locales.PlayerStart(), Disabled: disabled, Run: a.seekStart},
			{Label: locales.PlayerBack(), Disabled: disabled, Run: func() { a.seekBy(-15) }},
			{Label: locales.PlayerForward(), Disabled: disabled, Run: func() { a.seekBy(15) }},
			{Label: locales.PlayerEnd(), Disabled: disabled || a.duration <= 0, Run: a.seekEnd},
			{Divider: true},
			{Label: muteLabel, Run: func() { a.toggleMute(time.Now()) }},
		}
	})
}

// Progress owns the player's interaction until the task ends. All native
// playback entry points share this gate; unrelated busy operations do not.
func (a *NativeApp) playbackAvailable() bool {
	return a.mediaURL != "" && (!a.composeRunning || a.composeInBackground) && !a.recognitionRunning && !a.translationRunning
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

// Seeking to the beginning must not change play/pause, speed, or loop state.
func (a *NativeApp) seekStart() {
	if a.playbackAvailable() {
		a.evalPlayer("video.currentTime=0")
	}
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

// Pause before seeking so an enabled loop cannot restart at the end.
func (a *NativeApp) seekEnd() {
	if a.playbackAvailable() && a.duration > 0 {
		a.position, a.paused = a.duration, true
		a.evalPlayer("if (Number.isFinite(video.duration)) { video.pause(); video.currentTime=video.duration; }")
	}
}

func (a *NativeApp) setPlayerFullscreen(full bool) {
	if full == a.playerFullscreen || full && (!a.playbackAvailable() || a.composing || !a.commitEdit(true)) {
		return
	}
	a.playerFullscreen = full
	a.editing = false
	a.pendingClick, a.rowClick = nil, nil
	if a.win != nil {
		a.win.SetFullScreen(full)
	}
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
