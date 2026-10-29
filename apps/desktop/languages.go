package main

import (
	"embed"
	"strings"
	"sync"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
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
// Chinese, not detected. So there is no automatic option.
func sourceLanguages() []languageOption {
	return []languageOption{
		{"zh", "cn", locales.LanguageChina() + "-ZH"}, {"en", "gb", locales.LanguageBritain() + "-EN"},
		{"yue", "cn", locales.LanguageCantonese() + "-YUE"}, {"ja", "jp", locales.LanguageJapan() + "-JA"},
		{"ko", "kr", locales.LanguageKorea() + "-KO"}, {"de", "de", locales.LanguageGermany() + "-DE"},
		{"fr", "fr", locales.LanguageFrance() + "-FR"}, {"es", "es", locales.LanguageSpain() + "-ES"},
		{"it", "it", locales.LanguageItaly() + "-IT"}, {"pt", "pt", locales.LanguagePortugal() + "-PT"},
		{"ru", "ru", locales.LanguageRussia() + "-RU"},
	}
}
func targetLanguages() []languageOption {
	return []languageOption{
		{"zh-CN", "cn", locales.LanguageChina() + "-ZH-CN"}, {"en", "gb", locales.LanguageBritain() + "-EN"},
		{"ja", "jp", locales.LanguageJapan() + "-JA"}, {"ko", "kr", locales.LanguageKorea() + "-KO"},
		{"zh-TW", "cn", locales.LanguageTaiwan() + "-ZH-TW"},
	}
}

// languageSelect is a select of languages, each shown as its flag, then
// its name and code, in the choice and in the options. MyGo's SelectBase
// keeps the select's keyboard, popup and accessibility; it is given the
// look ui.Select has in the theme, with the app's Lucide chevron.
func languageSelect(c *ui.Context, value *string, options []languageOption, disabled bool) ui.Element {
	t := c.Theme()
	sel := ui.SelectBase(c, value)
	trigger := sel.Trigger.Disabled(disabled)
	trigger.Padding(t.Space(1.5), t.Space(3.5)).Gap(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).TextColor(t.Text).AlignItems(ui.Center)
	switch {
	case disabled:
		trigger.Background(t.Surface)
	case trigger.Pressed():
		trigger.Background(t.SurfacePressed)
	case trigger.Hovered():
		trigger.Background(t.SurfaceHover)
	default:
		trigger.Background(t.Surface)
	}
	chosen := languageOption{ID: *value, Label: *value}
	for _, o := range options {
		if o.ID == *value {
			chosen = o
		}
	}
	trigger.Children(func() {
		languageLabel(c, chosen).Grow(1)
		ui.Icon(c, icons["chevron-down"]).Size(em(c, 14), em(c, 14)).TextColor(t.TextMuted).Shrink(0)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).Radius(t.Radius+2).Background(t.Background).Border(1, t.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		for _, o := range options {
			item := sel.Item(o.ID).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
			switch {
			case item.Highlighted():
				item.Background(t.Accent).TextColor(t.AccentText)
			case o.ID == *value:
				item.Background(t.Surface)
			}
			item.Children(func() { languageLabel(c, o) })
		}
	})
	return trigger
}

// languageLabel is a language's flag, in the 4:3 of flag-icons, before its
// name and code.
func languageLabel(c *ui.Context, o languageOption) ui.Element {
	return ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).MinWidth(0).Children(func() {
		if flag := languageFlags()[o.Country]; flag != nil {
			ui.Image(c, flag).Size(em(c, 20), em(c, 15)).Shrink(0)
		}
		ui.Text(c, o.Label).SingleLine()
	})
}
