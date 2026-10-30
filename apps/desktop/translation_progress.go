package main

import (
	"fmt"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// translationLive is what a running translation has shown so far: its
// stage, the subtitles being translated and each one's text as the model
// writes it.
type translationLive struct {
	stage                   string // translator stages, then "check"
	done, total             int
	units                   []subtitle.Cue
	texts                   map[int]string
	final                   map[int]bool
	thinking, retry         string
	requests, input, output int
	latest                  int // the row last written to
	shown                   int // the row the list last followed
	list                    ui.ListState
	started                 time.Time
	stopped, model          string
	resume                  *translator.ResumeState
}

func newTranslationLive() *translationLive {
	return &translationLive{texts: map[int]string{}, final: map[int]bool{}, latest: -1, shown: -1, started: time.Now()}
}

func (l *translationLive) apply(ev translator.Event) {
	if ev.Resume != nil {
		r := *ev.Resume
		r.Completed = map[int]string{}
		for i, text := range ev.Resume.Completed {
			r.Completed[i] = text
		}
		l.resume = &r
	}
	l.stage, l.done, l.total = ev.Stage, ev.Done, ev.Total
	l.requests, l.input, l.output = ev.Requests, ev.Input, ev.Output
	if ev.Units != nil {
		l.units = ev.Units
	}
	for _, u := range ev.Updates {
		l.texts[u.Index] = u.Text
		l.final[u.Index] = u.Final
		if l.resume != nil && u.Final {
			l.resume.Completed[u.Index] = u.Text
		}
		l.latest = max(l.latest, u.Index)
	}
	if ev.Thinking != "" {
		l.thinking = ev.Thinking
	}
	if ev.Retry != "" {
		l.retry = ev.Retry
	} else if len(ev.Updates) > 0 {
		l.retry = ""
	}
}

// label says the stage a translation is in, with its count.
func (l *translationLive) label() string {
	switch l.stage {
	case "analyze":
		return locales.TranslationAnalyzing()
	case "segment":
		return locales.TranslationSegmenting(l.done, l.total)
	case "translate":
		return locales.TranslationProgress(l.done, l.total)
	case "check":
		return locales.TranslationChecking()
	}
	return locales.TranslationAnalyzing()
}

// translationProgressView takes the list's place while a translation runs:
// its stage, progress and use of the service, the model's reasoning when it
// shows any, and every subtitle with its translation as it is written. The
// project's subtitles are left alone until the whole file is translated and
// checked.
func (a *NativeApp) translationProgressView(c *ui.Context) {
	l := a.translationLive
	label := l.label()
	if l.stopped != "" {
		label = locales.TranslationInterrupted()
	}
	ui.Column(c).Padding(em(c, 8), em(c, 12)).Gap(em(c, 6)).Shrink(0).Label(locales.TranslationRunning()).Children(func() {
		ui.Row(c).Gap(em(c, 12)).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontFeatures("tnum").SingleLine()
			muted(c, locales.TranslationUsage(l.requests, l.input, l.output)).FontFeatures("tnum")
			if l.stopped != "" {
				ui.Button(c, locales.TranslationResume()).Disabled(a.busy).OnClick(a.retryTranslation)
			} else {
				ui.Button(c, locales.ActionCancelTask()).Disabled(a.cancel == nil).OnClick(func() {
					if a.cancel != nil {
						a.cancel()
					}
				})
			}
		})
		fraction := float64(-1)
		if (l.stage == "translate" || l.stage == "segment") && l.total > 0 {
			fraction = float64(l.done) / float64(l.total)
		}
		ui.Progress(c, fraction).Label(label)
		// What the model is doing now: a retry and why, its reasoning, or
		// the wait for its first words.
		note := l.retry
		if l.stopped != "" {
			note = locales.TranslationResumeNote()
		}
		if note == "" && l.thinking != "" {
			note = locales.TranslationThinking(l.thinking)
		}
		if note == "" && l.stopped == "" {
			note = locales.TranslationWaiting(int(time.Since(l.started).Seconds()))
			c.After(time.Second)
		}
		text := muted(c, note).SingleLine().Tooltip(note)
		if l.retry != "" {
			text.TextColor(c.Theme().Warning)
		}
		if l.model != "" {
			muted(c, l.model).SingleLine().Tooltip(l.model)
		}
		if l.stopped != "" {
			ui.Text(c, l.stopped).TextColor(c.Theme().Warning)
		}
	})
	ui.Divider(c)
	if len(l.units) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() { muted(c, label) })
		return
	}
	cols := []ui.TableColumn{numberColumn(c, len(l.units)), timeColumn(c, locales.ListColumnStart()), {Title: locales.TabSource()}, {Title: locales.TabTranslation()}}
	ui.Table(c, &l.list, cols, len(l.units), func(row, col int) {
		cue := l.units[row]
		switch col {
		case 0:
			muted(c, fmt.Sprint(row+1)).FontFeatures("tnum").SingleLine()
		case 1:
			muted(c, stamp(float64(cue.StartMs)/1000)).FontFeatures("tnum").SingleLine()
		case 2:
			muted(c, subtitle.SourceText(cue))
		default:
			// Accepted text in the normal color; text still being written
			// muted, as it may change.
			if l.final[row] {
				ui.Text(c, l.texts[row])
			} else {
				muted(c, l.texts[row])
			}
		}
	}).Grow(1).Label(locales.TranslationLive())
	// The list follows the newest translated row.
	if l.latest > l.shown {
		l.shown = l.latest
		l.list.ScrollTo(l.latest, ui.Center)
	}
}

func (a *NativeApp) showsTranslationProgress() bool {
	return a.subtitleTab == 1 && a.translationLive != nil && (a.translationRunning || a.translationLive.stopped != "")
}
