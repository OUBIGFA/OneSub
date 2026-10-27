package main

import (
	"fmt"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/egoist/mygo/ui"
)

// A shared section rhythm separates settings without nested cards. Labels stay
// with their fields; headings and whitespace identify each functional group.
func settingsGroup(c *ui.Context, title string, content func()) {
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(16).FontWeight(600)
		ui.Divider(c)
		content()
	})
}

func (a *NativeApp) settingsContent(c *ui.Context) {
	switch a.settingsTab {
	case 0:
		settingsGroup(c, "窗口", func() {
			behavior := "直接退出"
			if a.settings.CloseToTray {
				behavior = "最小化到托盘"
			}
			ui.Form(c, func() {
				ui.Field(c, "关闭窗口", func() {
					ui.Select(c, &behavior, []string{"直接退出", "最小化到托盘"}).Width(260).OnChange(func() { a.settings.CloseToTray = behavior == "最小化到托盘"; a.saveSettings() })
				})
			})
		})
		settingsGroup(c, "字幕与播放", func() {
			ui.Form(c, func() {
				a.playbackActionField(c, "单击字幕", &a.settings.SingleClickAction)
				a.playbackActionField(c, "双击字幕", &a.settings.DoubleClickAction)
			})
			ui.Checkbox(c, &a.settings.FollowPlayback, "播放时定位并滚动字幕").Margin(8, 0, 0, 0).OnChange(func() { a.followSuspended = false; a.activeCue = -1; a.saveSettings() })
		})
	case 1:
		settingsGroup(c, "模型选择", func() {
			ui.Grid(c).Columns(2).Gap(20).Children(func() {
				a.modelChoice(c, "识别模型", "asr", &a.settings.AsrModel)
				a.modelChoice(c, "对齐模型", "aligner", &a.settings.Aligner)
			})
		})
		settingsGroup(c, "识别参数", func() {
			ui.Grid(c).Columns(2).Gap(20).Children(func() {
				ui.Form(c, func() {
					ui.Field(c, "运行后端", func() { ui.Select(c, &a.settings.Backend, []string{"auto", "gpu", "cpu"}).OnChange(a.saveSettings) })
					ui.Field(c, "分块时长（秒）", func() { ui.NumberInput(c, &a.settings.ChunkSeconds, 30, 180, 1).OnChange(a.saveSettings) })
				})
				ui.Form(c, func() {
					ui.Field(c, "源语言", func() {
						ui.Select(c, &a.settings.Language, []string{"zh", "en", "yue", "ja", "ko", "de", "fr", "es", "it", "pt", "ru"}).OnChange(a.saveSettings)
					})
					ui.Field(c, "最大 Token 数", func() { ui.NumberInput(c, &a.settings.MaxTokens, 1, 32768, 1).OnChange(a.saveSettings) })
				})
			})
		})
		settingsGroup(c, "音频与热词", func() {
			ui.Checkbox(c, &a.settings.VocalSeparation, "人声分离").OnChange(a.saveSettings)
			ui.Field(c, "热词", func() { ui.TextArea(c, &a.settings.Hotwords).Height(52).OnChange(a.saveSettings) })
		})
	case 2:
		catalog := models.Catalog()
		if a.modelSelection == "" {
			a.modelSelection = a.settings.AsrModel
		}
		labels, selected := []string{}, ""
		var model models.Model
		for _, m := range catalog {
			label := fmt.Sprintf("%s · %.2f GB", m.Label, float64(m.Size())/1e9)
			labels = append(labels, label)
			if m.ID == a.modelSelection {
				selected = label
				model = m
			}
		}
		settingsGroup(c, "下载模型", func() {
			ui.Select(c, &selected, labels).Label("下载模型").OnChange(func() {
				for i, label := range labels {
					if selected == label {
						a.modelSelection = catalog[i].ID
						break
					}
				}
			})
		})
		settingsGroup(c, "模型文件", func() {
			ui.Form(c, func() {
				ui.Field(c, "存储目录", func() { ui.Text(c, a.modelPath(model.ID)).Selectable() })
				state := a.modelStates[model.ID]
				if state == "" {
					state = "未校验"
				}
				ui.Field(c, "文件状态", func() { ui.Text(c, state) })
			})
			ui.Row(c).Gap(12).Margin(8, 0, 0, 0).Children(func() {
				iconButton(c, "下载", "download").Disabled(a.busy).OnClick(func() { a.downloadModel(model) })
				ui.Button(c, "选择目录").Disabled(a.busy).OnClick(func() { a.pickModelDir(model) })
				ui.Button(c, "校验文件").Disabled(a.busy).OnClick(func() { a.verifyModel(model) })
			})
		})
	case 3:
		a.translationSettingsView(c)
	}
}
