package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// styledDocument is the only renderer input path for both preview and compose.
// Imported ASS retains its layout; ordinary subtitles use SmartSub's builder.
func (a *NativeApp) styledDocument(ctx context.Context, state project.Snapshot, style, secondary styles.Style, options subtitle.OutputOptions, frameAspect float64) (styles.Document, error) {
	document := state.OutputDocument(options)
	selected, err := subtitle.SelectOutput(document, options)
	if err != nil {
		return styles.Document{}, err
	}
	if state.Document.Format == "ass" || state.Document.Format == "ssa" {
		data, err := a.worker.Write(ctx, selected)
		if err != nil {
			return styles.Document{}, err
		}
		return a.styleClient.Resolve(ctx, string(data), style)
	}
	return a.styleClient.BuildOutput(ctx, document, style, secondary, options, frameAspect)
}

// Compare the complete style, not a hand-maintained list of fields. Every native
// control and preset therefore gets the same automatic preview semantics.
func (a *NativeApp) observeStyle(now time.Time) {
	if len(a.styleCatalog.Presets) == 0 {
		return
	}
	raw, _ := json.Marshal([]any{a.currentStyle, a.secondaryStyle, a.settings.SubtitleOutput})
	if string(raw) == a.observedStyle {
		return
	}
	a.observedStyle = string(raw)
	// Store the whole SmartSub style, so newly added parameters are remembered
	// too. Debounce disk I/O independently from the faster preview refresh.
	style := a.currentStyle
	a.settings.LastStyle = &style
	a.settings.StylePreset = a.stylePreset
	secondary := a.secondaryStyle
	a.settings.SecondaryStyle = &secondary
	a.settings.SecondaryPreset = a.secondaryPreset
	a.prefsSaveDue = now.Add(400 * time.Millisecond)
	a.previewRevision++
	a.previewPending = true
	a.previewDue = now.Add(140 * time.Millisecond)
}

func (a *NativeApp) refreshSubtitles() {
	if a.snapshot != nil && len(a.snapshot.OutputDocument(a.settings.SubtitleOutput).Cues) == 0 {
		a.previewPending = false
		a.evalPlayer("setSubtitles(null)")
		return
	}
	if a.snapshot == nil || a.mediaURL == "" || !a.playerReady || len(a.snapshot.Document.Cues) == 0 {
		a.previewPending = false
		return
	}
	state, style, version, aspect := *a.snapshot, a.currentStyle, a.previewRevision, a.frameAspect
	secondary, options := a.secondaryStyle, a.settings.SubtitleOutput
	a.previewPending, a.previewRunning = false, true
	ctx, cancel := context.WithCancel(context.Background())
	a.previewCancel = cancel
	// Preview has its own latest-wins lifetime. It must never put the whole
	// workbench into the busy state or disable controls while a slider moves.
	go func() {
		document, err := a.styledDocument(ctx, state, style, secondary, options, aspect)
		if err == nil && len(document.Report.MissingGlyphs) > 0 {
			err = errors.New(locales.ErrMissingGlyphs(strings.Join(document.Report.MissingGlyphs, " ")))
		}
		var fonts []string
		if err == nil {
			fonts, err = a.preview.RegisterFonts(ctx, document)
		}
		var payload []byte
		if err == nil {
			fontData, _ := json.Marshal(document.Fonts)
			payload, err = json.Marshal(map[string]any{"ass": document.ASS, "fonts": fonts, "defaultFont": document.Style.FontName, "fontKey": fmt.Sprintf("%x", sha256.Sum256(fontData))})
		}
		cancel()
		a.win.Update(func() {
			a.previewRunning = false
			a.previewCancel = nil
			if a.snapshot == nil || a.snapshot.Manifest.ID != state.Manifest.ID {
				return
			}
			if version != a.previewRevision || a.snapshot.Revision != state.Revision {
				a.previewPending = true
				return
			}
			if err != nil {
				a.showError("preview", err.Error())
				return
			}
			a.clearError("preview")
			a.evalPlayer("setSubtitles(" + string(payload) + ")")
		})
	}()
}
