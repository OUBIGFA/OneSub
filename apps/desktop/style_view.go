package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

func (a *NativeApp) stylePanel(c *ui.Context) {
	if a.snapshot == nil || len(a.snapshot.Document.Cues) == 0 {
		ui.Column(c).Fill().Padding(18, 20).Background(c.Theme().Surface).Children(func() {
			ui.Text(c, locales.StyleTitle()).FontWeight(600)
			ui.Column(c).Grow(1).Center().Children(func() { ui.Text(c, locales.ListEmpty()).TextColor(c.Theme().TextMuted) })
		})
		return
	}
	ui.Scroll(c).Fill().Background(c.Theme().Surface).Children(func() {
		ui.Column(c).Padding(18, 20).Gap(12).Children(func() {
			ui.Row(c).Height(34).AlignItems(ui.Center).Children(func() { ui.Text(c, locales.StyleTitle()).FontWeight(600) })
			a.styleView(c)
		})
	})
}

func inlineNumber(c *ui.Context, label string, value *float64, low, high, step float64) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).SingleLine()
		ui.NumberInput(c, value, low, high, step).Label(label)
	})
}
func styleColor(c *ui.Context, label string, value *string) {
	color := ui.Hex(*value)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).SingleLine()
		ui.ColorWell(c, &color).Label(label).OnChange(func() { *value = fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B) })
	})
}

// Flat native sections associate controls with their group, without custom drawing.
func styleSection(c *ui.Context, label string, content func()) {
	ui.Divider(c)
	ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).Width(64).FontWeight(600).TextColor(c.Theme().TextMuted).Padding(6, 0)
		ui.Column(c).Grow(1).Gap(10).Children(content)
	})
}

func (a *NativeApp) styleView(c *ui.Context) {
	if len(a.styleCatalog.Presets) == 0 {
		ui.Text(c, locales.StyleLoading()).TextColor(c.Theme().TextMuted)
		return
	}
	a.outputFields(c)
	if a.snapshot != nil && (a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa") {
		ui.Text(c, locales.StyleKeepAss()).TextColor(c.Theme().TextMuted)
		return
	}
	bilingual := a.settings.SubtitleOutput.Normalized().Mode == "bilingual"
	if bilingual {
		ui.Tabs(c, &a.styleLayer, locales.StylePrimaryLayer(), locales.StyleSecondaryLayer()).Label(locales.A11yStyleLayers())
	}
	a.styleControls(c, a.editedLayer())
}

// editedLayer is 1 while the secondary style of bilingual output is shown.
func (a *NativeApp) editedLayer() int {
	if a.settings.SubtitleOutput.Normalized().Mode == "bilingual" && a.styleLayer == 1 {
		return 1
	}
	return 0
}
func (a *NativeApp) layerStyle(layer int) (*styles.Style, *string) {
	if layer == 1 {
		return &a.secondaryStyle, &a.secondaryPreset
	}
	return &a.currentStyle, &a.stylePreset
}

// presetName says SmartSub's built-in presets in the app's language.
func presetName(p styles.Preset) string {
	switch p.ID {
	case "bilibili_knowledge":
		return locales.PresetBilibili()
	case "netflix_bilingual":
		return locales.PresetNetflix()
	case "variety_glow":
		return locales.PresetVariety()
	case "classic":
		return locales.PresetClassic()
	case "movie":
		return locales.PresetMovie()
	case "youtube":
		return locales.PresetYoutube()
	case "black_box":
		return locales.PresetBlackBox()
	case "clean":
		return locales.PresetClean()
	case "bold_impact":
		return locales.PresetBold()
	}
	return p.Name
}

// presetOptions lists the default style, SmartSub's presets and the user's
// own copies. A style that matches none of them is shown as custom.
func (a *NativeApp) presetOptions(current string) []option {
	options := []option{{"default", locales.PresetDefault()}}
	for _, p := range a.styleCatalog.Presets {
		options = append(options, option{p.ID, presetName(p)})
	}
	for _, p := range a.settings.UserPresets {
		options = append(options, option{p.ID, p.Name})
	}
	for _, o := range options {
		if o.ID == current {
			return options
		}
	}
	return append(options, option{current, locales.PresetCustom()})
}
func (a *NativeApp) presetLabel(id string) string {
	for _, o := range a.presetOptions(id) {
		if o.ID == id {
			return o.Label
		}
	}
	return locales.PresetCustom()
}
func (a *NativeApp) presetStyle(id string) (styles.Style, bool) {
	if id == "default" {
		return a.styleCatalog.DefaultStyle, true
	}
	for _, p := range a.styleCatalog.Presets {
		if p.ID == id {
			return p.Style, true
		}
	}
	for _, p := range a.settings.UserPresets {
		if p.ID == id {
			return p.Style, true
		}
	}
	return styles.Style{}, false
}
func (a *NativeApp) userPreset(id string) int {
	for i, p := range a.settings.UserPresets {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// Both layers edit SmartSub's existing schema. Bilingual block placement is
// shared; secondary typography and appearance remain independently editable.
func (a *NativeApp) styleControls(c *ui.Context, layer int) {
	style, preset := a.layerStyle(layer)
	fonts := []string{}
	for _, font := range a.styleCatalog.Fonts {
		if font.Available {
			fonts = append(fonts, font.Name)
		}
	}
	ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.StylePreset()).Width(64)
		idSelect(c, preset, a.presetOptions(*preset)).Width(200).Label(locales.StylePreset()).OnChange(func() {
			if chosen, ok := a.presetStyle(*preset); ok {
				*style = chosen
			}
		})
		// Saving always makes a new preset; SmartSub's presets and earlier
		// copies stay as they are.
		ui.Button(c, locales.PresetSaveNew()).OnClick(func() {
			a.presetDialog, a.presetDialogLayer = "save", layer
			a.presetName = locales.PresetNewName(len(a.settings.UserPresets) + 1)
		})
		own := a.userPreset(*preset) >= 0
		ui.Button(c, locales.PresetRename()).Disabled(!own).OnClick(func() {
			a.presetDialog, a.presetDialogLayer = "rename", layer
			a.presetName = a.settings.UserPresets[a.userPreset(*preset)].Name
		})
		ui.Button(c, locales.PresetDelete()).Disabled(!own).OnClick(func() { a.presetDialog, a.presetDialogLayer = "delete", layer })
	})
	styleSection(c, locales.StyleText(), func() {
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Combobox(c, &style.FontName, fonts).Width(200).Label(locales.StyleFont())
			inlineNumber(c, locales.StyleFontSize(), &style.FontSize, 10, 72, 1)
			styleColor(c, locales.StyleTextColor(), &style.PrimaryColor)
			ui.Button(c, locales.StyleReloadFonts()).Disabled(a.fontCatalogLoading).OnClick(a.reloadFonts)
		})
		ui.Row(c).Wrap().Gap(20).Children(func() {
			ui.Checkbox(c, &style.Bold, locales.StyleBold())
			ui.Checkbox(c, &style.Italic, locales.StyleItalic())
			ui.Checkbox(c, &style.Underline, locales.StyleUnderline())
		})
	})
	styleSection(c, locales.StyleAppearance(), func() {
		// Outline and box each show only their own parameters. In SmartSub's
		// mapping the back colour is the shadow in outline mode and the box
		// in box mode, so it is named after what it paints.
		border := strconv.Itoa(style.BorderStyle)
		box := style.BorderStyle == 3
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			idSelect(c, &border, []option{{"1", locales.StyleBorderOutline()}, {"3", locales.StyleBorderBox()}}).Width(130).Label(locales.StyleBorder()).OnChange(func() {
				style.BorderStyle, _ = strconv.Atoi(border)
			})
			if box {
				inlineNumber(c, locales.StyleBoxPadding(), &style.Outline, 0, 10, .1)
			} else {
				inlineNumber(c, locales.StyleOutlineWidth(), &style.Outline, 0, 10, .1)
				styleColor(c, locales.StyleOutlineColor(), &style.OutlineColor)
			}
		})
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			if box {
				styleColor(c, locales.StyleBoxColor(), &style.BackColor)
				inlineNumber(c, locales.StyleBoxOpacity(), &style.BackOpacity, 0, 100, 1)
			} else {
				inlineNumber(c, locales.StyleShadowDistance(), &style.Shadow, 0, 10, .1)
				styleColor(c, locales.StyleShadowColor(), &style.BackColor)
				inlineNumber(c, locales.StyleShadowOpacity(), &style.BackOpacity, 0, 100, 1)
			}
		})
	})
	if layer == 1 {
		ui.Text(c, locales.StyleSecondaryLayout()).TextColor(c.Theme().TextMuted)
		return
	}
	styleSection(c, locales.StyleLayout(), func() {
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			inlineNumber(c, locales.StyleMarginLeft(), &style.MarginL, 0, 384, 1)
			inlineNumber(c, locales.StyleMarginRight(), &style.MarginR, 0, 384, 1)
			inlineNumber(c, locales.StyleMarginVertical(), &style.MarginV, 0, 288, 1)
			alignment := strconv.Itoa(style.Alignment)
			ui.Text(c, locales.StylePosition()).SingleLine()
			idSelect(c, &alignment, alignmentOptions()).Width(130).Label(locales.StylePosition()).OnChange(func() {
				style.Alignment, _ = strconv.Atoi(alignment)
			})
		})
	})
}

// alignmentOptions are the nine ASS numpad positions.
func alignmentOptions() []option {
	labels := []string{locales.AlignBottomLeft(), locales.AlignBottomCenter(), locales.AlignBottomRight(), locales.AlignMiddleLeft(), locales.AlignCenter(), locales.AlignMiddleRight(), locales.AlignTopLeft(), locales.AlignTopCenter(), locales.AlignTopRight()}
	options := make([]option, len(labels))
	for i, label := range labels {
		options[i] = option{strconv.Itoa(i + 1), label}
	}
	return options
}

// presetDialogView names a new or renamed preset, or confirms a deletion.
func (a *NativeApp) presetDialogView(c *ui.Context) {
	open := a.presetDialog != ""
	ui.Modal(c, &open, func() {
		style, preset := a.layerStyle(a.presetDialogLayer)
		index := a.userPreset(*preset)
		ui.Column(c).Width(440).Gap(14).Children(func() {
			switch a.presetDialog {
			case "delete":
				if index < 0 {
					a.presetDialog = ""
					return
				}
				ui.Text(c, locales.PresetDeleteTitle(a.settings.UserPresets[index].Name)).FontSize(18).FontWeight(600)
				ui.Row(c).Gap(12).Justify(ui.End).Children(func() {
					ui.Button(c, locales.ActionCancel()).OnClick(func() { a.presetDialog = "" })
					ui.PrimaryButton(c, locales.PresetDeleteConfirm()).OnClick(func() {
						// The current values stay; only the saved copy goes.
						a.settings.UserPresets = append(a.settings.UserPresets[:index], a.settings.UserPresets[index+1:]...)
						*preset = "custom"
						a.presetDialog = ""
						a.saveSettings()
					})
				})
			default:
				title, confirm := locales.PresetSaveNew(), locales.PresetSaveConfirm()
				if a.presetDialog == "rename" {
					title, confirm = locales.PresetRename(), locales.PresetRenameConfirm()
				}
				ui.Text(c, title).FontSize(18).FontWeight(600)
				ui.TextInput(c, &a.presetName).Label(locales.PresetNameLabel()).Placeholder(locales.PresetNameLabel())
				name := strings.TrimSpace(a.presetName)
				ui.Row(c).Gap(12).Justify(ui.End).Children(func() {
					ui.Button(c, locales.ActionCancel()).OnClick(func() { a.presetDialog = "" })
					ui.PrimaryButton(c, confirm).Disabled(name == "").OnClick(func() {
						if a.presetDialog == "rename" && index >= 0 {
							a.settings.UserPresets[index].Name = name
						} else {
							id := "user-" + uuid.NewString()
							a.settings.UserPresets = append(a.settings.UserPresets, settings.UserPreset{ID: id, Name: name, Style: *style})
							*preset = id
						}
						a.presetDialog = ""
						a.saveSettings()
					})
				})
			}
		})
	})
	if !open {
		a.presetDialog = ""
	}
}
