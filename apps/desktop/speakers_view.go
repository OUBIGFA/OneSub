package main

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

type speakerUI struct {
	open           bool
	selected, name string
	candidate      *subtitle.SpeakerState
	project        string
	revision       int64
	ids            []string
	preview        []subtitle.Cue
	previewList    ui.ListState
}

func speakerText(cue subtitle.Cue, state *subtitle.SpeakerState) string {
	if cue.Speaker == nil {
		return ""
	}
	label := subtitle.SpeakerLabel(cue, state)
	if label == "" {
		label = locales.SpeakerUnknown()
		if cue.Speaker.Status == "mixed" || cue.Speaker.Status == "overlap" {
			label = locales.SpeakerMixed()
		}
	}
	if cue.Speaker.Review {
		label += " · " + locales.SpeakerReview()
	}
	return label
}

func (a *NativeApp) openSpeakers() {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	if a.speakers.project != a.snapshot.Manifest.ID {
		a.speakers.candidate = nil
		a.speakers.preview = nil
		a.speakers.selected = ""
		a.speakers.name = ""
	}
	a.speakers.selected, a.speakers.name = "", ""
	a.speakers.open = true
	a.speakers.ids = a.checkedRetranslationIDs()
	if len(a.speakers.ids) == 0 && a.selected >= 0 && a.selected < len(a.activeDocument().Cues) {
		a.speakers.ids = []string{a.activeDocument().Cues[a.selected].ID}
	}
	if a.selected >= 0 && a.selected < len(a.activeDocument().Cues) {
		cue := a.activeDocument().Cues[a.selected]
		if cue.Speaker != nil && len(cue.Speaker.IDs) == 1 {
			a.speakers.selected = cue.Speaker.IDs[0]
			if a.snapshot.Document.Speakers != nil {
				a.speakers.name = a.snapshot.Document.Speakers.Names[a.speakers.selected]
			}
		}
	}

	a.speakers.project = a.snapshot.Manifest.ID
}

func (a *NativeApp) speakerChange(change func() (int64, error)) {
	if a.store == nil || a.busy || !a.commitEdit(true) {
		return
	}
	if _, err := change(); err != nil {
		a.showError("speakers", err.Error())
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.showError("speakers", err.Error())
		return
	}
	a.setSnapshot(state)
	a.loadDraft()
}

func (a *NativeApp) analyzeCurrentSpeakers() {
	if a.snapshot == nil || a.snapshot.Manifest.MediaPath == "" || !a.commitEdit(true) {
		return
	}
	prefs := a.recognitionPreferences()
	source := a.snapshot.Manifest.MediaPath
	revision := a.snapshot.Revision
	projectID := a.snapshot.Manifest.ID
	t := a.toolset()
	a.run(locales.SpeakerAnnotate(), func(ctx context.Context) (func(), error) {
		state, err := t.analyzeSpeakers(ctx, prefs, source, "", func(recognitionStep) {})
		if err != nil {
			return nil, err
		}
		return func() {
			a.speakers.candidate = state
			a.speakers.project = projectID
			a.speakers.revision = revision
			a.speakers.preview = subtitle.AttachSpeakers(a.activeDocument(), state).Cues
		}, nil
	})
}

func (a *NativeApp) downloadSpeakerModels() {
	prefs := a.recognitionPreferences()
	t := a.toolset()
	a.run(locales.SpeakerDownload(), func(ctx context.Context) (func(), error) {
		for _, id := range []string{"speaker-segmentation", "speaker-embedding"} {
			model, err := models.ByID(id)
			if err != nil {
				return nil, err
			}
			if err = models.Download(ctx, &http.Client{Timeout: 30 * time.Minute}, model, t.modelPath(prefs, id), func(models.Progress) {}); err != nil {
				return nil, err
			}
		}
		return func() { a.refreshDownloadedAsrModels() }, nil
	})
}

func (a *NativeApp) speakersView(c *ui.Context) {
	f := &a.speakers
	fixedDialog(c, &f.open, locales.SpeakerTitle(), 620, 0, func() {
		a.speakerSettings(c)
		ui.Row(c).Gap(em(c, 12)).Wrap().Children(func() {
			lobeButton(c, locales.SpeakerDownload()).Disabled(a.busy).OnClick(a.downloadSpeakerModels)
			lobeButton(c, locales.SpeakerAnnotate()).Disabled(a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "").OnClick(a.analyzeCurrentSpeakers)
		})
		// Both states participate in layout so completion does not move controls.
		variant := 0
		if f.candidate != nil {
			variant = 1
		}
		stableVariants(c, variant, func() { muted(c, locales.SpeakerSelection()) }, func() { muted(c, locales.SpeakerPreview()) })
		ui.List(c, &f.previewList, len(f.preview), func(index int) {
			cue := f.preview[index]
			ui.Row(c).Gap(em(c, 8)).Children(func() {
				ui.Text(c, stamp(float64(cue.StartMs)/1000)).FontFeatures("tnum").Shrink(0)
				ui.Text(c, speakerText(cue, f.candidate)).MinWidth(em(c, 90))
				ui.Text(c, subtitle.SourceText(cue)).SingleLine().MinWidth(0).Grow(1).Tooltip(subtitle.SourceText(cue))
			})
		}).Height(em(c, 110)).MinHeight(em(c, 110)).Label(locales.SpeakerTitle())
		options := []option{{"", locales.SpeakerUnknown()}}
		var state *subtitle.SpeakerState
		if a.snapshot != nil {
			state = a.snapshot.Document.Speakers
		}
		if state != nil {
			ids := []string{}
			for id := range state.Names {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			for _, id := range ids {
				options = append(options, option{id, state.Names[id]})
			}
		}
		ui.Field(c, locales.SpeakerAssign(), func() {
			idSelect(c, &f.selected, options).Disabled(a.busy).Label(locales.SpeakerTitle()).OnChange(func() {
				if state != nil {
					f.name = state.Names[f.selected]
				}
			})
		})
		ui.Field(c, locales.SpeakerName(), func() { lobeTextInput(c, &f.name).Label(locales.SpeakerName()).Disabled(a.busy) })
		ui.Row(c).Gap(em(c, 12)).Wrap().Children(func() {
			lobeButton(c, locales.SpeakerAssign()).Disabled(a.busy || len(f.ids) == 0 || state == nil).OnClick(func() {
				a.speakerChange(func() (int64, error) {
					return a.store.SetSpeakers(uuid.NewString(), a.snapshot.Revision, f.ids, f.selected, a.subtitleTab == 0)
				})
			})
			lobeButton(c, locales.SpeakerRename()).Disabled(a.busy || f.selected == "" || f.name == "").OnClick(func() {
				a.speakerChange(func() (int64, error) {
					return a.store.RenameSpeaker(uuid.NewString(), a.snapshot.Revision, f.selected, f.name)
				})
			})
			lobeButton(c, locales.SpeakerAdd()).Disabled(a.busy || f.name == "" || a.snapshot == nil).OnClick(func() {
				a.speakerChange(func() (int64, error) { return a.store.AddSpeaker(uuid.NewString(), a.snapshot.Revision, f.name) })
			})
		})
	}, func() {
		lobeButton(c, locales.ActionClose()).OnClick(func() { f.open = false })
		ui.Spacer(c)
		lobePrimaryButton(c, locales.SpeakerApply()).Disabled(a.busy || f.candidate == nil).OnClick(func() {
			a.speakerChange(func() (int64, error) {
				if a.snapshot.Manifest.ID != f.project {
					return 0, errors.New(locales.TranslationResumeStale())
				}
				revision, err := a.store.ApplySpeakers(uuid.NewString(), f.revision, f.candidate)
				if err == nil {
					f.candidate = nil
					f.preview = nil
				}
				return revision, err
			})
		})
	})
}

func (a *NativeApp) speakerSettings(c *ui.Context) {
	lobeCheckbox(c, &a.settings.Diarize, locales.SpeakerAnnotate()).Disabled(a.busy).OnChange(a.saveSettings)
	ui.Field(c, locales.SpeakerCount(), func() {
		lobeNumberInput(c, &a.settings.SpeakerCount, 0, 100, 1, a.busy, a.saveSettings).Label(locales.SpeakerCount())
	})
}
