package main

import (
	"context"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

type listTextMatch struct {
	Cue int
	ID  string
	subtitle.TextMatch
}
type findReplaceState struct {
	open                   bool
	find, replacement      string
	matchCase, source      bool
	current                *listTextMatch
	cacheSnapshot          *project.Snapshot
	cacheFind              string
	cacheCase, cacheSource bool
	cacheTab               int
	matches                []listTextMatch
}

func (a *NativeApp) textTarget(source bool) string {
	if a.subtitleTab == 0 {
		return "original"
	}
	if source {
		return "source"
	}
	return "translation"
}
func (a *NativeApp) textTargetLabel(source bool) string {
	if a.subtitleTab == 0 {
		return locales.TabOriginal()
	}
	if source {
		return locales.TabSource()
	}
	return locales.TabTranslation()
}
func listTargetText(cue subtitle.Cue, track string) (string, bool) {
	if cue.ReadOnly {
		return "", false
	}
	if track == "translation" {
		return cue.Text, cue.Translated
	}
	return subtitle.SourceText(cue), true
}

func (a *NativeApp) listPunctuationTools(c *ui.Context) {
	disabled := a.busy || a.showsTranslationProgress() || a.snapshot == nil
	hint := locales.TextProcessScope(a.textTargetLabel(a.editRebuiltSource))
	toolIcon(c, locales.TextReplacePeriods(), "quote").Disabled(disabled).Tooltip(locales.TextReplacePeriods() + "\n" + hint).OnClick(func() { a.cleanListPunctuation(false) })
	toolIcon(c, locales.TextStripTrailing(), "eraser").Disabled(disabled).Tooltip(locales.TextStripTrailing() + "\n" + hint).OnClick(func() { a.cleanListPunctuation(true) })
}

func (a *NativeApp) openFindReplace() {
	if a.busy || a.snapshot == nil || a.showsTranslationProgress() || !a.commitEdit(true) {
		return
	}
	f := &a.findReplace
	f.open, f.source, f.current = true, a.editRebuiltSource, nil
	if a.query != "" {
		f.find = a.query
	}
}

func (a *NativeApp) findMatches() []listTextMatch {
	f := &a.findReplace
	if f.cacheSnapshot == a.snapshot && f.cacheFind == f.find && f.cacheCase == f.matchCase && f.cacheSource == f.source && f.cacheTab == a.subtitleTab && f.matches != nil {
		return f.matches
	}
	f.cacheSnapshot, f.cacheFind, f.cacheCase, f.cacheSource, f.cacheTab = a.snapshot, f.find, f.matchCase, f.source, a.subtitleTab
	f.matches = []listTextMatch{}
	finder := subtitle.NewLiteralFinder(f.find, f.matchCase)
	track := a.textTarget(f.source)
	for i, cue := range a.activeDocument().Cues {
		text, ok := listTargetText(cue, track)
		if !ok {
			continue
		}
		for _, m := range finder.Matches(text) {
			f.matches = append(f.matches, listTextMatch{i, cue.ID, m})
		}
	}
	return f.matches
}

func (a *NativeApp) findNextText() {
	matches := a.findMatches()
	if len(matches) == 0 {
		a.findReplace.current = nil
		return
	}
	next := 0
	for i, m := range matches {
		current := a.findReplace.current
		if current == nil {
			if m.Cue >= a.selected {
				next = i
				break
			}
		} else if m.Cue > current.Cue || m.ID == current.ID && m.Start >= current.End {
			next = i
			break
		}
	}
	m := matches[next]
	a.findReplace.current = &m
	a.selected, a.locateCue, a.query = m.Cue, m.Cue, ""
	a.pendingClick, a.rowClick = nil, nil
	a.list.StopScroll()
	a.followHeld = true
	if a.subtitleTab == 1 {
		a.editRebuiltSource = a.findReplace.source
	}
	a.loadDraft()
}

func currentMatchNumber(matches []listTextMatch, current listTextMatch) int {
	for i, match := range matches {
		if match == current {
			return i + 1
		}
	}
	return 0
}

func (a *NativeApp) findReplaceView(c *ui.Context) {
	f := &a.findReplace
	fixedDialog(c, &f.open, locales.FindReplace(), 640, 460, func() {
		ui.Field(c, locales.FindText(), func() {
			ui.TextInput(c, &f.find).Label(locales.FindText()).AutoFocus().Disabled(a.busy).OnChange(func() { f.current = nil })
		})
		ui.Field(c, locales.ReplaceWith(), func() { ui.TextInput(c, &f.replacement).Label(locales.ReplaceWith()).Disabled(a.busy) })
		ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c, &f.matchCase, locales.FindMatchCase()).Disabled(a.busy).OnChange(func() { f.current = nil })
			if a.subtitleTab == 1 {
				track := locales.TabTranslation()
				if f.source {
					track = locales.TabSource()
				}
				ui.Select(c, &track, []string{locales.TabSource(), locales.TabTranslation()}).Label(locales.FindScope()).MinWidth(em(c, 140)).Disabled(a.busy).OnChange(func() { f.source = track == locales.TabSource(); f.current = nil })
			} else {
				muted(c, locales.TabOriginal())
			}
		})
		matches := a.findMatches()
		text := locales.FindCount(len(matches))
		if f.current != nil {
			text = locales.FindCurrent(f.current.Cue+1, currentMatchNumber(matches, *f.current), len(matches))
		}
		ui.Text(c, text)
		muted(c, locales.FindLiteralHint())
	}, func() {
		ui.Button(c, locales.ActionClose()).OnClick(func() { f.open = false })
		disabled := a.busy || len(a.findMatches()) == 0
		ui.Button(c, locales.FindNext()).Disabled(disabled).OnClick(a.findNextText)
		ui.Button(c, locales.ReplaceOne()).Disabled(disabled).OnClick(func() { a.replaceListText(false) })
		ui.PrimaryButton(c, locales.ReplaceAll()).Disabled(disabled).OnClick(func() { a.replaceListText(true) })
	})
}

// The two manual text operations share the same validation/transaction path.
// Strict format preflight happens before any edit; a failure leaves the entire
// batch untouched. The subtitle file remains unchanged until explicit Save.
func (a *NativeApp) listTextEditWork(edits []project.Edit) func(context.Context) (func(), error) {
	// Freeze the target before starting a background task; switching tabs or
	// editing controls while it runs cannot redirect a batch to another track.
	doc, revision, store, worker := a.activeDocument(), a.snapshot.Revision, a.store, a.worker
	return func(ctx context.Context) (func(), error) {
		if len(edits) == 0 {
			return nil, nil
		}
		projected := doc
		projected.Cues = append([]subtitle.Cue{}, doc.Cues...)
		byID := map[string]project.Edit{}
		for _, e := range edits {
			byID[e.ID] = e
		}
		for i, cue := range projected.Cues {
			e, ok := byID[cue.ID]
			if !ok {
				continue
			}
			// Serialize the edited text layer, independently of the paired layer.
			projected.Cues[i].Text = e.Text
		}
		if doc.Format != "srt" {
			if _, err := worker.Write(ctx, projected); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := store.Apply(uuid.NewString(), revision, edits); err != nil {
			return nil, err
		}
		state, err := store.Snapshot()
		if err != nil {
			return nil, err
		}
		return func() {
			a.setSnapshot(state)
			a.loadDraft()
			a.followHeld = true
			a.pendingClick = nil
			a.splitSkipped = nil
		}, nil
	}
}

func (a *NativeApp) replacementEdits(all bool) ([]project.Edit, int, *listTextMatch) {
	f := &a.findReplace
	if len(a.findMatches()) == 0 {
		return nil, 0, nil
	}
	if !all && f.current == nil {
		a.findNextText()
	}
	track := a.textTarget(f.source)
	finder := subtitle.NewLiteralFinder(f.find, f.matchCase)
	edits := []project.Edit{}
	count := 0
	var next *listTextMatch
	for i, cue := range a.activeDocument().Cues {
		text, ok := listTargetText(cue, track)
		if !ok {
			continue
		}
		value, n := text, 0
		if all {
			value, n = finder.ReplaceAll(text, f.replacement)
		} else if f.current != nil && cue.ID == f.current.ID {
			m := f.current
			// A changed document invalidates a previous occurrence, rather than
			// letting a stale byte offset replace unrelated text.
			for _, found := range finder.Matches(text) {
				if found == m.TextMatch {
					value = text[:m.Start] + f.replacement + text[m.End:]
					n = 1
					end := m.Start + len(f.replacement)
					next = &listTextMatch{i, cue.ID, subtitle.TextMatch{Start: end, End: end}}
					break
				}
			}
		}
		if n > 0 && value != text {
			edits = append(edits, project.Edit{ID: cue.ID, Text: value, Track: track, StartMs: cue.StartMs, EndMs: cue.EndMs})
			count += n
		}
	}
	return edits, count, next
}

func (a *NativeApp) replaceListText(all bool) {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	edits, count, next := a.replacementEdits(all)
	f := &a.findReplace
	work := a.listTextEditWork(edits)
	a.run(locales.FindReplace(), func(ctx context.Context) (func(), error) {
		apply, err := work(ctx)
		if err != nil {
			return nil, err
		}
		return func() {
			if apply != nil {
				apply()
			}
			f.current = next
			a.status = locales.ReplaceResult(count, len(edits))
			if !all {
				a.findNextText()
			}
		}, nil
	})
}

func (a *NativeApp) punctuationEdits(strip bool) []project.Edit {
	options := subtitle.PunctuationOptions{StripTrailing: strip, ReplacePeriods: !strip, Keep: a.settings.KeepPunctuation}
	track := a.textTarget(a.editRebuiltSource)
	edits := []project.Edit{}
	for _, cue := range a.activeDocument().Cues {
		text, ok := listTargetText(cue, track)
		if !ok {
			continue
		}
		value := options.Apply(text)
		if value != text && strings.TrimSpace(value) != "" {
			edits = append(edits, project.Edit{ID: cue.ID, Text: value, Track: track, StartMs: cue.StartMs, EndMs: cue.EndMs})
		}
	}
	return edits
}

func (a *NativeApp) cleanListPunctuation(strip bool) {
	if a.busy || a.snapshot == nil || a.showsTranslationProgress() || !a.commitEdit(true) {
		return
	}
	edits := a.punctuationEdits(strip)
	label := locales.TextReplacePeriods()
	if strip {
		label = locales.TextStripTrailing()
	}
	work := a.listTextEditWork(edits)
	a.run(label, func(ctx context.Context) (func(), error) {
		apply, err := work(ctx)
		if err != nil {
			return nil, err
		}
		return func() {
			if apply != nil {
				apply()
			}
			a.status = locales.TextProcessResult(len(edits))
		}, nil
	})
}
