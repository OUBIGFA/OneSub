package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
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
	a.problems.glyphs = nil
	a.problems.glyphRevision++
	// Store the whole SmartSub style, so newly added parameters are remembered
	// too. Only disk I/O waits for input to stop; a held step button must
	// not keep postponing the preview. One in-flight build coalesces changes.
	style := a.currentStyle
	a.settings.LastStyle = &style
	a.settings.StylePreset = a.stylePreset
	secondary := a.secondaryStyle
	a.settings.SecondaryStyle = &secondary
	a.settings.SecondaryPreset = a.secondaryPreset
	a.prefsSaveDue = now.Add(400 * time.Millisecond)
	a.previewRevision++
	a.previewPending = true
	a.previewDue = now
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
	// A partial translation previews its completed rows without filling gaps
	// with source text. File output still requires the explicit skip choice.
	options.SkipMissing = options.Normalized().Mode != "source" && subtitle.MissingTranslations(state.Document) > 0
	a.previewPending, a.previewRunning = false, true
	ctx, cancel := context.WithCancel(context.Background())
	a.previewCancel = cancel
	// Preview has its own latest-wins lifetime. It must never put the whole
	// workbench into the busy state or disable controls while a slider moves.
	go func() {
		document, err := a.styledDocument(ctx, state, style, secondary, options, aspect)
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
			if a.snapshot.Revision != state.Revision {
				a.previewPending = true
				return
			}
			if version != a.previewRevision {
				a.previewPending = true
				if err != nil {
					return // an obsolete style must not report an error
				}
				// Builds run one at a time, so this cannot replace a newer
				// rendered result. Show the completed style while building
				// the next, rather than freezing during continuous input.
			}
			if err != nil {
				// A failed replacement must not leave the previous language or
				// document visible under the newly selected subtitle-content mode.
				a.evalPlayer("setSubtitles(null)")
				a.showError("preview", err.Error())
				return
			}
			if version == a.previewRevision {
				a.problems.glyphs = append([]string(nil), document.Report.MissingGlyphs...)
				a.problems.glyphRevision++
				// Report missing glyphs while showing the current subtitles. An
				// obsolete font/style result must not change the current warning.
				if len(document.Report.MissingGlyphs) > 0 {
					a.showError("preview", subtitleProblemReason(subtitle.CueProblem{Kind: "glyphs", Characters: strings.Join(document.Report.MissingGlyphs, "")}))
				} else {
					a.clearError("preview")
				}
			}
			a.evalPlayer("setSubtitles(" + string(payload) + ")")
		})
	}()
}
