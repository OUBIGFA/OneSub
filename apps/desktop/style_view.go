package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *NativeApp) stylePanel(c *ui.Context) {
	if a.snapshot == nil || len(a.snapshot.Document.Cues) == 0 {
		ui.Column(c).Fill().Padding(18, 20).Background(c.Theme().Surface).Children(func() {
			ui.Text(c, "字幕样式").FontWeight(600)
			ui.Column(c).Grow(1).Center().Children(func() { ui.Text(c, "未加载字幕").TextColor(c.Theme().TextMuted) })
		})
		return
	}
	ui.Scroll(c).Fill().Background(c.Theme().Surface).Children(func() {
		ui.Column(c).Padding(18, 20).Gap(12).Children(func() {
			ui.Row(c).Height(34).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "字幕样式").FontWeight(600)
			})
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

// Flat native sections associate each control with its group. No nested cards,
// custom borders or hand-painted controls are needed for this hierarchy.
func styleSection(c *ui.Context, label string, content func()) {
	ui.Divider(c)
	ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).Width(64).FontWeight(600).TextColor(c.Theme().TextMuted).Padding(6, 0)
		ui.Column(c).Grow(1).Gap(10).Children(content)
	})
}

func (a *NativeApp) styleView(c *ui.Context) {
	if len(a.styleCatalog.Presets) == 0 {
		ui.Text(c, "正在加载字体与样式").TextColor(c.Theme().TextMuted)
		return
	}
	if a.snapshot != nil && (a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa") {
		ui.Text(c, "保留 ASS 原有样式").TextColor(c.Theme().TextMuted)
		return
	}
	names, fonts := []string{"默认"}, []string{}
	for _, p := range a.styleCatalog.Presets {
		names = append(names, p.Name)
	}
	for _, font := range a.styleCatalog.Fonts {
		if font.Available {
			fonts = append(fonts, font.Name)
		}
	}
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "预设").Width(64)
		ui.Select(c, &a.stylePreset, names).Width(200).Label("样式预设").OnChange(func() {
			if a.stylePreset == "默认" {
				a.currentStyle = a.styleCatalog.DefaultStyle
			} else {
				for _, p := range a.styleCatalog.Presets {
					if p.Name == a.stylePreset {
						a.currentStyle = p.Style
						break
					}
				}
			}
		})
	})
	styleSection(c, "文字", func() {
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Combobox(c, &a.currentStyle.FontName, fonts).Width(200).Label("字幕字体")
			inlineNumber(c, "字号", &a.currentStyle.FontSize, 10, 72, 1)
			styleColor(c, "文字颜色", &a.currentStyle.PrimaryColor)
			ui.Button(c, "刷新字体").Disabled(a.fontCatalogLoading).OnClick(a.reloadFonts)
		})
		ui.Row(c).Wrap().Gap(20).Children(func() {
			ui.Checkbox(c, &a.currentStyle.Bold, "粗体")
			ui.Checkbox(c, &a.currentStyle.Italic, "斜体")
			ui.Checkbox(c, &a.currentStyle.Underline, "下划线")
		})
	})
	styleSection(c, "外观", func() {
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			border := "描边"
			if a.currentStyle.BorderStyle == 3 {
				border = "背景框"
			}
			ui.Select(c, &border, []string{"描边", "背景框"}).Width(110).Label("边框样式").OnChange(func() {
				a.currentStyle.BorderStyle = 1
				if border == "背景框" {
					a.currentStyle.BorderStyle = 3
				}
			})
			outlineLabel := "描边宽度"
			if a.currentStyle.BorderStyle == 3 {
				outlineLabel = "背景框边距"
			}
			inlineNumber(c, outlineLabel, &a.currentStyle.Outline, 0, 10, .1)
			if a.currentStyle.BorderStyle == 1 {
				inlineNumber(c, "阴影", &a.currentStyle.Shadow, 0, 10, .1)
			}
		})
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			if a.currentStyle.BorderStyle == 1 {
				styleColor(c, "描边颜色", &a.currentStyle.OutlineColor)
			}
			styleColor(c, "背景颜色", &a.currentStyle.BackColor)
			inlineNumber(c, "不透明度（%）", &a.currentStyle.BackOpacity, 0, 100, 1)
		})
	})
	styleSection(c, "布局", func() {
		ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			inlineNumber(c, "左边距", &a.currentStyle.MarginL, 0, 384, 1)
			inlineNumber(c, "右边距", &a.currentStyle.MarginR, 0, 384, 1)
			inlineNumber(c, "垂直边距", &a.currentStyle.MarginV, 0, 288, 1)
			alignments := []string{"左下", "居中下", "右下", "左中", "居中", "右中", "左上", "居中上", "右上"}
			alignment := "居中下"
			if a.currentStyle.Alignment >= 1 && a.currentStyle.Alignment <= 9 {
				alignment = alignments[a.currentStyle.Alignment-1]
			}
			ui.Select(c, &alignment, alignments).Width(110).Label("字幕位置").OnChange(func() {
				for i, name := range alignments {
					if name == alignment {
						a.currentStyle.Alignment = i + 1
						break
					}
				}
			})
		})
	})
}
