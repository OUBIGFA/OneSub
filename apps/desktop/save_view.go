package main

import "github.com/egoist/mygo/ui"

// guardChanges defers the original action, not just its dialog. Choosing Save
// commits first; failed writes and Cancel keep both the draft and current view.
func (a *NativeApp) guardChanges(next func()) bool {
	if !a.dirty {
		return false
	}
	if !a.savePromptOpen {
		a.afterSave = next
		a.savePromptOpen = true
	}
	return true
}

func (a *NativeApp) finishSavePrompt(choice string) {
	if choice == "save" && !a.saveDraft() {
		return
	}
	next := a.afterSave
	a.savePromptOpen, a.afterSave = false, nil
	if choice == "cancel" {
		return
	}
	if choice == "discard" {
		a.loadDraft()
		a.status = "已放弃修改"
	}
	if next != nil {
		next()
	}
}

func (a *NativeApp) savePromptView(c *ui.Context) {
	ui.Modal(c, &a.savePromptOpen, func() {
		ui.Text(c, "是否保存字幕修改？").FontSize(18).FontWeight(600)
		ui.Text(c, "当前字幕有未保存的修改")
		ui.Row(c).Gap(12).Children(func() {
			ui.Button(c, "取消").OnClick(func() { a.finishSavePrompt("cancel") })
			ui.Button(c, "不保存").OnClick(func() { a.finishSavePrompt("discard") })
			ui.PrimaryButton(c, "保存修改").Disabled(a.composing).OnClick(func() { a.finishSavePrompt("save") })
		})
	})
}

func (a *NativeApp) changeSubtitleTab(tab int) {
	if a.guardChanges(func() { a.changeSubtitleTab(tab) }) {
		return
	}
	a.subtitleTab = tab
	a.loadDraft()
}

func (a *NativeApp) selectSubtitle(index int, action string) {
	if a.guardChanges(func() { a.selectSubtitle(index, action) }) {
		return
	}
	a.selected = index
	a.loadDraft()
	a.seekSubtitle(action)
}

func (a *NativeApp) requestExport() {
	if a.guardChanges(a.requestExport) {
		return
	}
	a.exportOpen = true
}

func (a *NativeApp) hideToTray() {
	if a.guardChanges(a.hideToTray) {
		return
	}
	a.win.Hide()
}
