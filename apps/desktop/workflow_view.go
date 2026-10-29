package main

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// workflowPanel is where subtitles are made: speech recognition with its
// options and output, and AI translation, each a tab ending in the one
// button that starts it. The services' accounts and models stay in the
// settings, a click away.
func (a *NativeApp) workflowPanel(c *ui.Context) {
	ui.Column(c).Fill().Label(locales.WorkflowTitle()).Children(func() {
		// The two tasks switch in the header, as the list's original and
		// translation do.
		panelHeader(c, func() { panelTitle(c, locales.WorkflowTitle()) }, func() {
			ui.Segmented(c, &a.workflowTab, locales.WorkflowRecognize(), locales.WorkflowTranslate()).Label(locales.WorkflowTitle())
		}, nil)
		ui.Divider(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(em(c, 12), em(c, panelPad)).Gap(em(c, 12)).Children(func() {
				if a.workflowTab == 0 {
					a.recognizeTab(c)
				} else {
					a.translateTab(c)
				}
			})
		})
	})
}

// recognizeTab starts with what it does and to which file, then the
// options of a recognition and of the file it saves. Labels sit above their
// fields, so the panel stays narrow.
func (a *NativeApp) recognizeTab(c *ui.Context) {
	hasMedia := a.snapshot != nil && a.snapshot.Manifest.MediaPath != ""
	startButton(c, locales.WorkflowStartRecognize(), "audio-lines", a.busy || !hasMedia, a.recognize)
	if hasMedia {
		name := filepath.Base(a.snapshot.Manifest.MediaPath)
		muted(c, name).SingleLine().Tooltip(name).AlignSelf(ui.Center)
	} else {
		muted(c, locales.WorkflowNeedsVideo()).AlignSelf(ui.Center)
	}
	ui.Divider(c)
	ui.Field(c, locales.SettingsSourceLanguage(), func() {
		languageSelect(c, &a.settings.Language, sourceLanguages(), a.busy).Label(locales.SettingsSourceLanguage()).OnChange(a.saveSettings)
	})
	ui.Row(c).Gap(em(c, 12)).Wrap().Children(func() {
		ui.Field(c, locales.SettingsBackend(), func() {
			idSelect(c, &a.settings.Backend, []option{{"auto", locales.BackendAuto()}, {"gpu", "GPU"}, {"cpu", "CPU"}}).Label(locales.SettingsBackend()).OnChange(a.saveSettings)
		})
		ui.Field(c, locales.ExportFormat(), func() {
			idSelect(c, &a.settings.RecognitionFormat, []option{{"srt", "SRT"}, {"vtt", "VTT"}, {"ass", "ASS"}}).Label(locales.ExportFormat()).OnChange(a.saveSettings)
		})
	})
	ui.Row(c).Gap(em(c, 12)).Wrap().Children(func() {
		ui.Field(c, locales.SettingsChunk(), func() {
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
				ui.NumberInput(c, &a.settings.ChunkSeconds, 30, 180, 1).Label(locales.SettingsChunk()).OnChange(a.saveSettings)
				muted(c, locales.UnitSeconds()).SingleLine()
			})
		})
		ui.Field(c, locales.SettingsMaxTokens(), func() {
			ui.NumberInput(c, &a.settings.MaxTokens, 1, 32768, 1).Label(locales.SettingsMaxTokens()).AlignSelf(ui.Start).OnChange(a.saveSettings)
		})
	})
	ui.Field(c, locales.RecognitionDirectory(), func() {
		path := a.settings.RecognitionDir
		if path == "" {
			path = locales.RecognitionBesideMedia()
		}
		ui.Column(c).Gap(em(c, 6)).Children(func() {
			muted(c, path).SingleLine().Tooltip(path)
			ui.Row(c).Gap(em(c, 8)).Wrap().Children(func() {
				ui.Button(c, locales.RecognitionChooseFolder()).Disabled(a.busy).OnClick(a.pickRecognitionDir)
				ui.Button(c, locales.RecognitionResetFolder()).Disabled(a.busy || a.settings.RecognitionDir == "").OnClick(func() { a.settings.RecognitionDir = ""; a.saveSettings() })
			})
		})
	})
	ui.Checkbox(c, &a.settings.StripPunctuation, locales.WorkflowStripPunctuation()).OnChange(a.saveSettings)
	ui.Field(c, locales.WorkflowKeepPunctuation(), func() {
		ui.TextInput(c, &a.settings.KeepPunctuation).Placeholder("?").Label(locales.WorkflowKeepPunctuation()).Disabled(!a.settings.StripPunctuation).OnChange(a.saveSettings)
	})
	ui.Button(c, locales.WorkflowModels()).AlignSelf(ui.Start).OnClick(func() { a.settingsOpen, a.settingsTab = true, 1 })
}

// translateTab starts with its button, then the language and service it
// translates with.
func (a *NativeApp) translateTab(c *ui.Context) {
	hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
	startButton(c, locales.WorkflowStartTranslate(), "languages", a.busy || !hasCues, a.translate)
	if !hasCues {
		muted(c, locales.WorkflowNeedsSubtitles()).AlignSelf(ui.Center)
	}
	ui.Divider(c)
	ui.Field(c, locales.TranslationTarget(), func() {
		languageSelect(c, &a.settings.TargetLanguage, targetLanguages(), a.busy).Label(locales.TranslationTarget()).OnChange(a.saveSettings)
	})
	ui.Field(c, locales.TranslationModel(), func() {
		// No service yet is the one thing to fix first: it says so.
		if model := a.settings.ProviderModel; model != "" {
			muted(c, model).SingleLine().Tooltip(model)
		} else {
			ui.Text(c, locales.WorkflowNoService()).TextColor(c.Theme().Warning)
		}
	})
	ui.Checkbox(c, &a.settings.OptimizeTimeline, locales.TranslationOptimizeTimeline()).OnChange(a.saveSettings)
	ui.Button(c, locales.WorkflowService()).AlignSelf(ui.Start).OnClick(func() { a.reloadTranslationProfiles(); a.settingsOpen, a.settingsTab = true, 2 })
}

// startButton is a panel's one primary action: full width, its icon and a
// verb that says the task starts.
func startButton(c *ui.Context, label, name string, disabled bool, run func()) {
	ui.PrimaryButton(c, "").Label(label).Tooltip(label).Justify(ui.Center).Padding(em(c, 10), em(c, 16)).Disabled(disabled).OnClick(run).Children(func() {
		icon(c, name)
		ui.Text(c, label).SingleLine()
	})
}

// pairedMarks are the ASCII and full-width forms of the same mark: keeping
// one keeps both.
var pairedMarks = map[rune]rune{'?': '？', '!': '！', ',': '，', '.': '。', ';': '；', ':': '：', '？': '?', '！': '!', '，': ',', '。': '.', '；': ';', '：': ':'}

// stripTrailingPunctuation removes punctuation at the end of each line of
// text, except the marks in keep, in either width. Closing quotes and
// brackets stay, as removing them would leave their opening half alone.
func stripTrailingPunctuation(text, keep string) string {
	kept := map[rune]bool{}
	for _, r := range keep {
		kept[r] = true
		if p, ok := pairedMarks[r]; ok {
			kept[p] = true
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		runes := []rune(strings.TrimRight(line, " \t\r"))
		end := len(runes)
		for end > 0 {
			r := runes[end-1]
			if !unicode.IsPunct(r) || kept[r] || unicode.In(r, unicode.Pe, unicode.Pf) || r == '"' || r == '\'' {
				break
			}
			end--
		}
		if end > 0 {
			lines[i] = string(runes[:end])
		} else {
			lines[i] = strings.TrimRight(line, " \t\r")
		}
	}
	return strings.Join(lines, "\n")
}
