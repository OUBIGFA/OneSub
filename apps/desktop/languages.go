package main

import (
	"embed"
	"slices"
	"strings"
	"sync"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	lobeField "github.com/lobehub/lobe-mygo/pkg/field"
)

// Flags are unmodified flag-icons assets, drawn in their own colors by
// ui.Image. They identify a representative country or region, not a
// restriction on who speaks the selected language.
//
//go:embed flags/*.svg
var flagFiles embed.FS
var languageFlags = sync.OnceValue(func() map[string]*ui.SVG {
	result := map[string]*ui.SVG{}
	entries, err := flagFiles.ReadDir("flags")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		data, err := flagFiles.ReadFile("flags/" + entry.Name())
		if err != nil {
			panic(err)
		}
		result[strings.TrimSuffix(entry.Name(), ".svg")] = ui.MustParseSVG(data)
	}
	return result
})

type languageOption struct{ ID, Country, Label string }

// OneAsr v1.3.0 requires an explicit language throughout ASR + alignment:
// its forced aligner covers these 11 languages, and "auto" is read as
// Chinese, not detected. The sherpa-onnx models recognize the subsets in
// their catalog entries; "auto" is SenseVoiceSmall's own detection. Smart
// routing (see routeRecognition) needs no language or model at all.
func sourceLanguages() []languageOption {
	return []languageOption{
		{languageAuto, "", locales.LanguageAuto()},
		{"zh", "cn", locales.LanguageChina() + "-ZH"}, {"en", "gb", locales.LanguageBritain() + "-EN"},
		{"yue", "cn", locales.LanguageCantonese() + "-YUE"}, {"ja", "jp", locales.LanguageJapan() + "-JA"},
		{"ko", "kr", locales.LanguageKorea() + "-KO"}, {"de", "de", locales.LanguageGermany() + "-DE"},
		{"fr", "fr", locales.LanguageFrance() + "-FR"}, {"es", "es", locales.LanguageSpain() + "-ES"},
		{"it", "it", locales.LanguageItaly() + "-IT"}, {"pt", "pt", locales.LanguagePortugal() + "-PT"},
		{"ru", "ru", locales.LanguageRussia() + "-RU"},
	}
}

// modelLanguages are the source languages that the ASR model id
// recognizes, in the order of sourceLanguages.
func modelLanguages(id string) []languageOption {
	model, err := models.ByID(id)
	if err != nil {
		return sourceLanguages()
	}
	out := []languageOption{}
	for _, o := range sourceLanguages() {
		// "auto" only for a model that detects the language itself.
		if model.Supports(o.ID) {
			out = append(out, o)
		}
	}
	return out
}

// asrModelChanged restores this model's language, or defaults a first choice
// to its own detection when supported. It never resets a later user override.
func (a *NativeApp) asrModelChanged() {
	languages := modelLanguages(a.settings.AsrModel)
	if len(languages) == 0 {
		return
	}
	language, remembered := a.settings.ManualLanguages[a.settings.AsrModel]
	supported := func(id string) bool {
		return slices.ContainsFunc(languages, func(o languageOption) bool { return o.ID == id })
	}
	switch {
	case remembered && supported(language):
		a.settings.Language = language
	case supported(languageAuto):
		a.settings.Language = languageAuto
	case !supported(a.settings.Language):
		a.settings.Language = languages[0].ID
	}
	a.sourceLanguageChanged()
}

// A manual language change belongs to the current model, not to the next
// model selected. Store it before saving so a later return preserves it.
func (a *NativeApp) sourceLanguageChanged() {
	if a.settings.ManualLanguages == nil {
		a.settings.ManualLanguages = map[string]string{}
	}
	a.settings.ManualLanguages[a.settings.AsrModel] = a.settings.Language
	a.saveSettings()
}

func targetLanguages() []languageOption {
	return []languageOption{
		{"zh-CN", "cn", locales.LanguageChina() + "-ZH-CN"}, {"en", "gb", locales.LanguageBritain() + "-EN"},
		{"ja", "jp", locales.LanguageJapan() + "-JA"}, {"ko", "kr", locales.LanguageKorea() + "-KO"},
		{"zh-TW", "cn", locales.LanguageTaiwan() + "-ZH-TW"},
	}
}

// Lobe owns the selector, popup and keyboard interaction. Its label slot
// retains the registered flag assets and complete localized language names.
func languageSelect(c *ui.Context, value *string, options []languageOption, disabled bool) ui.Element {
	items := make([]lobeField.SelectOption, len(options))
	byID := make(map[string]languageOption, len(options))
	for i, option := range options {
		items[i] = lobeField.SelectOption{Value: option.ID, Label: option.Label}
		byID[option.ID] = option
	}
	return lobeField.Select(c, value, lobeField.SelectProps{
		Options: items, Disabled: disabled, Placeholder: *value,
		RenderLabel: func(option lobeField.SelectOption) { languageLabel(c, byID[option.Value]).Grow(1) },
	})
}

// languageLabel is a language's flag, in the 4:3 of flag-icons, before its
// name and code.
func languageLabel(c *ui.Context, o languageOption) ui.Element {
	return ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).MinWidth(0).Children(func() {
		if flag := languageFlags()[o.Country]; flag != nil {
			ui.Image(c, flag).Size(em(c, 20), em(c, 15)).Shrink(0)
		}
		// Language names are fixed product labels, not abbreviatable user
		// content. A narrow inspector wraps the complete name and code.
		ui.Text(c, o.Label).MinWidth(0)
	})
}
