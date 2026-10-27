package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

type translationCandidate struct {
	ProjectID string              `json:"projectId"`
	Revision  int64               `json:"baseRevision"`
	Result    translator.Result   `json:"result"`
	Checks    translator.Findings `json:"checks"`
}

func (a *NativeApp) translationSettingsView(c *ui.Context) {
	settingsGroup(c, "服务连接", func() {
		ui.Form(c, func() {
			ui.Field(c, "服务地址", func() {
				address := ui.TextInput(c, &a.settings.ProviderURL).Placeholder("https://服务地址/v1").OnChange(func() {
					a.providerKey = ""
					a.keyStatus = ""
					a.providerModels = nil
					a.modelsError = ""
					a.saveSettings()
				})
				// Wait until editing ends; do not query partial hostnames as they are typed.
				if !address.Focused() && a.settings.ProviderURL != a.modelsEndpoint {
					a.fetchProviderModels()
				}
			})
			ui.Field(c, "模型", func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Combobox(c, &a.settings.ProviderModel, a.providerModels).Placeholder("选择或填写模型名称").Grow(1).OnChange(a.saveSettings)
					ui.Button(c, "获取模型").Disabled(a.modelsLoading || a.settings.ProviderURL == "").OnClick(a.fetchProviderModels)
				})
			})
			if a.modelsLoading {
				ui.Text(c, "正在获取模型列表").TextColor(c.Theme().TextMuted)
			}
			if a.modelsError != "" {
				ui.Text(c, a.modelsError).TextColor(c.Theme().Warning)
			}
			ui.Field(c, "API Key", func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.providerKey).Password().Placeholder("留空不修改已存 Key").Grow(1)
					ui.Button(c, "保存 Key").Disabled(a.providerKey == "").OnClick(func() {
						if err := security.SaveKey(a.settings.ProviderURL, a.providerKey); err != nil {
							a.errorText = err.Error()
							return
						}
						a.providerKey = ""
						a.keyStatus = "已存入系统凭据"
						a.modelsEndpoint = ""
						a.fetchProviderModels()
					})
					ui.Button(c, "删除 Key").OnClick(func() {
						if err := security.DeleteKey(a.settings.ProviderURL); err != nil {
							a.errorText = err.Error()
							return
						}
						a.providerKey = ""
						a.keyStatus = "Key 已删除"
					})
				})
			})
			if a.keyStatus != "" {
				ui.Field(c, "", func() { ui.Text(c, a.keyStatus).TextColor(c.Theme().TextMuted) })
			}
		})
	})
	settingsGroup(c, "翻译选项", func() {
		ui.Grid(c).Columns(2).Gap(20).Children(func() {
			ui.Field(c, "目标语言", func() {
				ui.Select(c, &a.settings.TargetLanguage, []string{"zh-CN", "en", "ja", "ko", "zh-TW"}).OnChange(a.saveSettings)
			})
			ui.Field(c, "API 请求上限", func() { ui.NumberInput(c, &a.settings.MaxRequests, 1, 10000, 1).OnChange(a.saveSettings) })
		})
	})
	settingsGroup(c, "术语", func() {
		ui.Checkbox(c, &a.settings.Domain3D, "3D / 动效 / VFX 术语").OnChange(a.saveSettings)
		ui.TextArea(c, &a.settings.Glossary).Height(52).Label("自定义术语").Placeholder("原词 = 译词").OnChange(a.saveSettings)
	})
}

func (a *NativeApp) translationView(c *ui.Context) {
	ui.Modal(c, &a.translationConfirm, func() {
		ui.Text(c, "翻译字幕")
		ui.Text(c, a.settings.ProviderModel)
		ui.Text(c, a.settings.ProviderURL)
		if a.snapshot != nil {
			total, locked := 0, 0
			for _, cue := range a.snapshot.Document.Cues {
				if cue.ReadOnly || cue.TextLocked {
					locked++
				} else {
					total++
				}
			}
			ui.Text(c, fmt.Sprintf("%d 条 · %s · 跳过 %d 条", total, a.settings.TargetLanguage, locked))
		}
		ui.Text(c, "仅发送字幕和术语，不发送音视频")
		ui.Text(c, fmt.Sprintf("最多 %d 次请求；服务单价未知，费用以服务账单为准", int(a.settings.MaxRequests))).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "取消").OnClick(func() { a.translationConfirm = false })
			ui.PrimaryButton(c, "开始翻译").OnClick(func() { a.translationConfirm = false; a.startTranslation() })
		})
	})
	ui.Modal(c, &a.translationReview, func() {
		candidate := a.translationCandidate
		if candidate == nil {
			return
		}
		ui.Text(c, "审阅译文")
		ui.Text(c, fmt.Sprintf("%d 条译文 · %d 项错误 · %d 项提醒", candidate.Result.Report.Translated, len(candidate.Checks.Errors), len(candidate.Checks.Warnings)+len(candidate.Result.Report.Warnings)))
		ui.Scroll(c).Height(130).Width(720).Children(func() {
			for _, item := range candidate.Checks.Errors {
				ui.Text(c, item).TextColor(c.Theme().Danger)
			}
			for _, item := range candidate.Result.Report.Warnings {
				ui.Text(c, item).TextColor(c.Theme().Warning)
			}
			for _, item := range candidate.Checks.Warnings {
				ui.Text(c, item).TextColor(c.Theme().Warning)
			}
			ui.Text(c, "保持原时间轴；语义与音画同步需核对").TextColor(c.Theme().TextMuted)
		})
		a.translationList.Selected = &a.translationSelected
		cues := candidate.Result.Document.Cues
		ui.Table(c, &a.translationList, []ui.TableColumn{numberColumn(c, len(cues)), {Title: "原文"}, {Title: "译文"}}, len(cues), func(row, col int) {
			cue := cues[row]
			text := cue.Text
			if col == 0 {
				text = fmt.Sprint(row + 1)
			} else if col == 1 {
				text = cue.OriginalText
			}
			ui.Text(c, text).SingleLine()
		}).Height(200).Label("译文审阅列表")
		if a.translationSelected >= 0 && a.translationSelected < len(cues) {
			ui.Text(c, cues[a.translationSelected].Text)
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "稍后核对").OnClick(func() { a.translationReview = false })
			ui.PrimaryButton(c, "接受译文").Disabled(len(candidate.Checks.Errors) > 0 || a.busy).OnClick(a.acceptTranslation)
		})
	})
}

func (a *NativeApp) startTranslation() {
	if a.snapshot == nil || a.guardChanges(a.startTranslation) {
		return
	}
	state := *a.snapshot
	prefs := a.settings
	key, err := security.ReadKey(prefs.ProviderURL)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	root := filepath.Join(a.resources, "srt-subtitle-translator")
	if value := os.Getenv("ONESUB_SKILL_ROOT"); value != "" {
		root = value
	}
	executable := filepath.Join(a.resources, "skill-tools", "OneSub.SkillTools.exe")
	if value := os.Getenv("ONESUB_SKILL_WORKER"); value != "" {
		executable = value
	}
	config := translator.Config{Endpoint: prefs.ProviderURL, Model: prefs.ProviderModel, Key: key, TargetLanguage: prefs.TargetLanguage, SkillRoot: root, Glossary: prefs.Glossary, Domain3D: prefs.Domain3D, MaxRequests: int(prefs.MaxRequests)}
	checker := translator.Checker{Executable: executable, SkillRoot: root, Scratch: a.worker.Scratch, Subtitle: a.worker}
	a.run("翻译字幕", func(ctx context.Context) (func(), error) {
		// Always translate the retained source, not a previous machine translation.
		// Locked hand edits stay in place and never enter the writable candidate set.
		source := state.Document
		source.Cues = append([]subtitle.Cue{}, source.Cues...)
		for i := range source.Cues {
			if !source.Cues[i].TextLocked && !source.Cues[i].ReadOnly && source.Cues[i].Translated {
				source.Cues[i].Text = cueDisplayText(source.Cues[i], 0)
			}
		}
		result, err := translator.Translate(ctx, config, source, func(done, total int) { a.win.Update(func() { a.status = fmt.Sprintf("翻译 %d / %d", done, total) }) })
		if err != nil {
			return nil, err
		}
		a.win.Update(func() { a.status = "检查翻译" })
		checks, err := checker.Check(ctx, source, result.Document, prefs.TargetLanguage)
		if err != nil {
			return nil, err
		}
		candidate := &translationCandidate{ProjectID: state.Manifest.ID, Revision: state.Revision, Result: result, Checks: checks}
		data, err := json.MarshalIndent(candidate, "", "  ")
		if err != nil {
			return nil, err
		}
		artifacts := filepath.Join(state.Path, "artifacts")
		if err = os.MkdirAll(artifacts, 0700); err != nil {
			return nil, err
		}
		if err = subtitle.Publish(filepath.Join(artifacts, "translation-"+uuid.NewString()+".json"), data); err != nil {
			return nil, err
		}
		return func() {
			a.translationCandidate = candidate
			a.translationReview = true
			a.translationSelected = 0
			a.status = "译文待核对"
		}, nil
	})
}

// Accepting the whole reviewed result is one undoable transaction. The store
// repeats revision/lock checks, so an old review cannot overwrite later edits.
func (a *NativeApp) acceptTranslation() {
	if a.snapshot == nil || a.translationCandidate == nil || a.guardChanges(a.acceptTranslation) {
		return
	}
	candidate := a.translationCandidate
	if candidate.ProjectID != a.snapshot.Manifest.ID || candidate.Revision != a.snapshot.Revision {
		a.errorText = "REVISION_CONFLICT：内容已修改，请重新翻译"
		return
	}
	if len(candidate.Checks.Errors) > 0 {
		a.errorText = "请先处理翻译检查错误"
		return
	}
	edits := []project.Edit{}
	for i, cue := range candidate.Result.Document.Cues {
		if cue.ReadOnly || cue.TextLocked {
			continue
		}
		if i >= len(a.snapshot.Document.Cues) || a.snapshot.Document.Cues[i].ID != cue.ID {
			a.errorText = "翻译结果与字幕 ID 不匹配"
			return
		}
		if cue.Text == a.snapshot.Document.Cues[i].Text && a.snapshot.Document.Cues[i].Translated {
			continue
		}
		edits = append(edits, project.Edit{ID: cue.ID, Text: cue.Text, StartMs: cue.StartMs, EndMs: cue.EndMs})
	}
	if len(edits) == 0 {
		a.errorText = "译文没有变化"
		return
	}
	_, err := a.store.ApplyTranslation(uuid.NewString(), candidate.Revision, edits)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return
	}
	a.snapshot = &state
	a.previewPending = true
	a.subtitleTab = 1
	a.loadDraft()
	a.translationReview = false
	a.translationCandidate = nil
	a.errorText = ""
	a.status = "译文已保存"
}

func (a *NativeApp) setTextLock(locked bool) {
	if a.snapshot == nil || a.selected < 0 || a.guardChanges(func() { a.setTextLock(locked) }) {
		return
	}
	if a.store == nil {
		a.errorText = "工程未打开"
		return
	}
	_, err := a.store.SetTextLock(uuid.NewString(), a.snapshot.Revision, a.snapshot.Document.Cues[a.selected].ID, locked)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return
	}
	a.snapshot = &state
	a.loadDraft()
	a.status = "已保存"
}
