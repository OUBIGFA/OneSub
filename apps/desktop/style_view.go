package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	lobeField "github.com/lobehub/lobe-mygo/pkg/field"
)

// stylePanel keeps one shared heading above independently scrolling panes.
// The left pane chooses content and presets; the right edits that layer's style.
func (a *NativeApp) stylePanel(c *ui.Context) {
	width, _ := c.Size()
	a.stylePanelAtWidth(c, width)
}

func (a *NativeApp) stylePanelAtWidth(c *ui.Context, width float32) {
	_, height := c.Size()
	a.stylePanelAtSize(c, width, height)
}

func (a *NativeApp) stylePanelAtSize(c *ui.Context, width, height float32) {
	hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
	ready := hasCues && len(a.styleCatalog.Presets) > 0
	ass := a.snapshot != nil && (a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa")
	bilingual := !ass && a.settings.SubtitleOutput.Normalized().Mode == "bilingual"
	panel := ui.Column(c).Fill().Label(locales.A11yStylePanel())
	split := ui.Local(panel, "content-width", func() float32 { return float32(a.settings.StyleContentWidth) })
	contentScroll := ui.Local(panel, "content-scroll", func() ui.ScrollState { return ui.ScrollState{} })
	// Use this frame's allocated width. Temporary viewport clamps must not
	// overwrite the saved ratio or wait for a measured second frame.
	fit := func(value float32) float32 {
		room, left, right := width-1, em(c, 220), em(c, 360)
		if bilingual {
			// Keep both full layer names visible in the horizontal selector.
			left = em(c, 320)
		}
		if room >= left+right {
			return min(100-right/room*100, max(left/room*100, value))
		}
		return value
	}
	shown := fit(*split)
	panel.Children(func() {
		panelHeader(c, nil, func() { panelTitle(c, locales.StyleTitle()) }, nil)
		separator(c)
		if height <= em(c, headerSize)+1.01 {
			return
		}
		switch {
		case !hasCues:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.ListEmpty()) })
		case !ready:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.StyleLoading()) })
		default:
			ui.SplitPercent(c, &shown, func() {
				ui.Scroll(c).Fill().TrackScroll(contentScroll).Children(func() {
					ui.Column(c).Padding(em(c, 16), em(c, panelPad)).Gap(em(c, 16)).Children(func() {
						if bilingual {
							segmentedFill(c, &a.styleLayer, locales.StylePrimaryLayer(), locales.StyleSecondaryLayer()).Label(locales.A11yStyleLayers())
						}
						layer := a.editedLayer()
						cells := a.outputFields(c, false)
						if !ass {
							cells = append(cells, func() {
								ui.Field(c, locales.StylePreset(), func() {
									ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() { a.presetControls(c, layer) })
								})
							})
						}
						for _, cell := range cells {
							cell()
						}
					})
				})
			}, func() {
				ui.Scroll(c).Fill().TrackScroll(&a.styleScroll).Children(func() {
					ui.Column(c).Padding(em(c, 16), em(c, panelPad)).Gap(em(c, 16)).Children(func() {
						if ass {
							muted(c, locales.StyleKeepAss())
							return
						}
						// Panel navigation uses tabs; this segmented choice switches
						// properties of the same subtitle style without another tab bar.
						segmentedFill(c, &a.styleTab, locales.StyleText(), locales.StyleAppearance(), locales.StyleLayout()).Label(locales.StyleProperties()).OnChange(func() { a.styleScroll.Y = 0 })
						// The selected native tab already groups these properties;
						// use the same flat field alignment as the other inspectors.
						ui.Column(c).Gap(em(c, 16)).Children(func() { a.styleControls(c, a.editedLayer()) })
					})
				})
			}).Grow(1).OnChange(func() {
				// Match saved pane bounds so a narrow drag survives a restart.
				*split = fit(min(90, max(10, shown)))
				a.settings.StyleContentWidth = float64(*split)
				a.prefsSaveDue = c.Now().Add(400 * time.Millisecond)
			})
		}
	})
}

// presetControls choose a preset; its actions are in one menu, as style
// menus of editors are. Saving always makes a new preset; SmartSub's presets
// and earlier copies stay as they are, and only the user's own are renamed
// or deleted.
func (a *NativeApp) presetControls(c *ui.Context, layer int) {
	style, preset := a.layerStyle(layer)
	idSelectWithProps(c, preset, a.presetOptions(*preset), lobeField.SelectProps{ShowAll: true}).Grow(1).MinWidth(0).Label(locales.StylePreset()).OnChange(func() {
		if chosen, ok := a.presetStyle(*preset); ok {
			*style = chosen
		}
	})
	ui.Toolbar(c, func() {
		lobeMenuButton(c, locales.PresetActions(), "ellipsis", true, func() []menuCommand {
			own := a.userPreset(*preset) >= 0
			original, known := a.presetStyle(*preset)
			return []menuCommand{
				{Label: locales.PresetRestoreDefaults(), Disabled: own || !known, Run: func() { *style = original }},
				{Label: locales.PresetSaveNew(), Run: func() {
					a.presetDialog, a.presetDialogLayer = "save", layer
					a.presetName = locales.PresetNewName(len(a.settings.UserPresets) + 1)
				}},
				{Label: locales.PresetRename(), Disabled: !own, Run: func() {
					a.presetDialog, a.presetDialogLayer = "rename", layer
					a.presetName = a.settings.UserPresets[a.userPreset(*preset)].Name
				}},
				{Label: locales.PresetDelete(), Disabled: !own, Danger: true, Run: func() { a.presetDialog, a.presetDialogLayer = "delete", layer }},
			}
		})
	}).Padding(0)
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

// colorWell edits a #RRGGBB color; its name is the property it paints.
func colorWell(c *ui.Context, label string, value *string) ui.Element {
	color := ui.Hex(*value)
	before := color
	picker := lobeColorPicker(c, &color).Label(label).AlignSelf(ui.Start)
	if color != before {
		*value = fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
	}
	return picker
}

// number is a native number input at its own size: a narrower one would
// draw its step buttons over what follows.
func number(c *ui.Context, label string, value *float64, low, high, step float64, changed ...func()) ui.Element {
	return lobeNumberInput(c, value, low, high, step, false, changed...).Label(label)
}

// percent is a number input for an opacity, with its unit after it.
func percent(c *ui.Context, label string, value *float64) {
	ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
		number(c, label, value, 0, 100, 1)
		muted(c, "%").SingleLine()
	})
}

// Font, appearance and placement are independent native inspector pages.
// Only the active page is built; no invisible controls influence its size.
func (a *NativeApp) styleControls(c *ui.Context, layer int) {
	style, _ := a.layerStyle(layer)
	switch a.styleTab {
	case 0:
		a.textSection(c, style)
	case 1:
		a.appearanceSection(c, style)
	case 2:
		if layer == 0 {
			a.layoutSection(c, style)
		} else {
			muted(c, locales.StyleSecondaryLayout())
		}
	}
}

func (a *NativeApp) textSection(c *ui.Context, style *styles.Style) {
	fonts := []string{}
	for _, font := range a.styleCatalog.Fonts {
		if font.Available {
			fonts = append(fonts, font.Name)
		}
	}
	ui.Field(c, locales.StyleFont(), func() {
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			lobeCombobox(c, &style.FontName, fonts, func(frame ui.Element) { frame.Grow(1).MinWidth(0) }).Label(locales.StyleFont())
			toolIcon(c, locales.StyleReloadFonts(), "rotate-cw").Disabled(a.fontCatalogLoading).OnClick(a.reloadFonts)
		})
	})
	parameterGrid(c, 160, 2, func() {
		ui.Field(c, locales.StyleFontSize(), func() { number(c, locales.StyleFontSize(), &style.FontSize, 1, 72, .5) })
	}, func() {
		ui.Field(c, locales.StyleColor(), func() { colorWell(c, locales.StyleTextColor(), &style.PrimaryColor) })
	})
	ui.Field(c, locales.StyleEmphasis(), func() {
		ui.ToggleGroup(c, func() {
			lobeToggle(c, &style.Bold, "").Grow(1).Basis(0).Label(locales.StyleBold()).Tooltip(locales.StyleBold()).Children(func() { icon(c, "bold") })
			lobeToggle(c, &style.Italic, "").Grow(1).Basis(0).Label(locales.StyleItalic()).Tooltip(locales.StyleItalic()).Children(func() { icon(c, "italic") })
			lobeToggle(c, &style.Underline, "").Grow(1).Basis(0).Label(locales.StyleUnderline()).Tooltip(locales.StyleUnderline()).Children(func() { icon(c, "underline") })
		}).Label(locales.StyleEmphasis())
	})
}

// boxOpacity is SmartSub's default opacity of the background box. A box
// chosen while the opacity is 0, OneSub's default for the shadow, gets it:
// a box no one can see is not what choosing one asks for.
const boxOpacity = 50

func (a *NativeApp) appearanceSection(c *ui.Context, style *styles.Style) {
	box := style.BorderStyle == 3
	border := 0
	if box {
		border = 1
	}
	ui.Field(c, locales.StyleBorder(), func() {
		if segmentedFill(c, &border, locales.StyleBorderOutline(), locales.StyleBorderBox()).Label(locales.StyleBorder()).Changed() {
			style.BorderStyle = 1
			if border == 1 {
				style.BorderStyle = 3
				if style.BackOpacity == 0 {
					style.BackOpacity = boxOpacity
				}
			}
		}
	})
	if box {
		parameterGrid(c, 210, 2, func() {
			ui.Field(c, locales.StylePadding(), func() { number(c, locales.StyleBoxPadding(), &style.Outline, 0, 10, .1) })
		}, func() {
			ui.Field(c, locales.StyleBackground(), func() { colorWell(c, locales.StyleBoxColor(), &style.BackColor) })
		}, func() {
			ui.Field(c, locales.StyleBoxOpacityShort(), func() { percent(c, locales.StyleBoxOpacity(), &style.BackOpacity) })
		})
		return
	}
	parameterGrid(c, 200, 2, func() {
		ui.Field(c, locales.StyleBorderOutline(), func() {
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
				colorWell(c, locales.StyleOutlineColor(), &style.OutlineColor)
				number(c, locales.StyleOutlineWidth(), &style.Outline, 0, 10, .1)
			})
		})
	}, func() {
		ui.Field(c, locales.StyleShadow(), func() {
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
				colorWell(c, locales.StyleShadowColor(), &style.BackColor)
				number(c, locales.StyleShadowDistance(), &style.Shadow, 0, 10, .1)
			})
		})
	}, func() {
		ui.Field(c, locales.StyleShadowOpacityShort(), func() { percent(c, locales.StyleShadowOpacity(), &style.BackOpacity) })
	})
}

func (a *NativeApp) layoutSection(c *ui.Context, style *styles.Style) {
	row, column := (style.Alignment-1)/3, (style.Alignment-1)%3
	ui.Field(c, locales.StylePosition(), func() {
		segmentedFill(c, &row, locales.PositionBottom(), locales.PositionMiddle(), locales.PositionTop()).Label(locales.StylePosition())
	})
	ui.Field(c, locales.StyleAlign(), func() {
		ui.ToggleGroup(c, func() {
			for i, name := range []string{"align-left", "align-center", "align-right"} {
				on := column == i
				label := []string{locales.AlignLeft(), locales.AlignCenterH(), locales.AlignRight()}[i]
				if lobeToggle(c, &on, "").Grow(1).Basis(0).Label(label).Tooltip(label).Children(func() { icon(c, name) }).Changed() {
					column = i
				}
			}
		}).Label(locales.StyleAlign())
	})
	style.Alignment = row*3 + column + 1
	separator(c)
	parameterGrid(c, 160, 3, func() {
		ui.Field(c, locales.StyleMarginLeft(), func() { number(c, locales.StyleMarginLeft(), &style.MarginL, 0, 384, 1) })
	}, func() {
		ui.Field(c, locales.StyleMarginRight(), func() { number(c, locales.StyleMarginRight(), &style.MarginR, 0, 384, 1) })
	}, func() {
		ui.Field(c, locales.StyleMarginVertical(), func() { number(c, locales.StyleMarginVertical(), &style.MarginV, 0, 288, 1) })
	})
}

// presetDialogView names a new or renamed preset, or confirms a deletion
// in an alert.
func (a *NativeApp) presetDialogView(c *ui.Context) {
	style, preset := a.layerStyle(a.presetDialogLayer)
	index := a.userPreset(*preset)
	if a.presetDialog == "delete" {
		open := index >= 0
		if !open {
			a.presetDialog = ""
			return
		}
		switch confirmationDialog(c, &open, locales.PresetDeleteTitle(a.settings.UserPresets[index].Name), locales.PresetDeleteBody(), locales.ActionCancel(), locales.PresetDeleteConfirm()) {
		case 1:
			// The current values stay; only the saved copy goes.
			a.settings.UserPresets = append(a.settings.UserPresets[:index], a.settings.UserPresets[index+1:]...)
			*preset = "custom"
			a.saveSettings()
		}
		if !open {
			a.presetDialog = ""
		}
		return
	}
	open := a.presetDialog != ""
	title, confirm := locales.PresetSaveNew(), locales.PresetSaveConfirm()
	if a.presetDialog == "rename" {
		title, confirm = locales.PresetRename(), locales.PresetRenameConfirm()
	}
	fixedDialog(c, &open, title, 480, 0, func() {
		ui.Field(c, locales.PresetNameLabel(), func() {
			lobeTextInput(c, &a.presetName).AutoFocus().Label(locales.PresetNameLabel()).Placeholder(locales.PresetNameLabel())
		})
	}, func() {
		name := strings.TrimSpace(a.presetName)
		lobeButton(c, locales.ActionCancel()).OnClick(func() { a.presetDialog = "" })
		lobePrimaryButton(c, confirm).Disabled(name == "").OnClick(func() {
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
	if !open {
		a.presetDialog = ""
	}
}
