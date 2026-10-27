package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var composeModes = []string{"硬字幕（烧录） · MP4", "硬字幕（烧录） · MKV", "软字幕（封装） · MKV", "软字幕（封装） · MP4"}

func (a *NativeApp) composeView(c *ui.Context) {
	ui.Modal(c, &a.composeOpen, func() {
		ui.Text(c, "合成视频").FontSize(18).FontWeight(600)
		ui.Form(c, func() {
			ui.Field(c, "字幕模式", func() { ui.Select(c, &a.composeMode, composeModes) })
			if strings.HasPrefix(a.composeMode, "硬字幕") {
				ui.Field(c, "编码质量", func() { ui.Select(c, &a.composeQuality, []string{"CRF 18", "CRF 20", "CRF 23"}) })
				ui.Text(c, "H.264 / libx264 · CRF 越低，质量越高、文件越大").TextColor(c.Theme().TextMuted)
			}
		})
		if a.composeMode == "软字幕（封装） · MP4" {
			ui.Text(c, "MP4 使用 mov_text 字幕，不保留 ASS 字体和排版").TextColor(c.Theme().Warning)
		}
		if a.currentStyle.FontName != "" {
			ui.Text(c, a.stylePreset+" · "+a.currentStyle.FontName)
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "取消").OnClick(func() { a.composeOpen = false })
			ui.Button(c, "导出 5 秒预览").OnClick(func() { a.composeOpen = false; a.startCompose(true) })
			ui.PrimaryButton(c, "开始合成").OnClick(func() { a.composeOpen = false; a.startCompose(false) })
		})
	})
}

// Progress occupies the player's media area, not a status-bar-only indicator.
// Preparation and full decode verification have unknown duration, never a fake %.
func (a *NativeApp) composeProgressView(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(18).Label("视频合成进度").Children(func() {
		ui.Text(c, "正在合成视频").FontSize(20).FontWeight(600)
		ui.Text(c, a.composeProgress.Stage).Label("合成阶段")
		ui.Progress(c, a.composeProgress.Fraction).Width(400).Label("合成进度")
		if a.composeProgress.Fraction >= 0 {
			ui.Text(c, fmt.Sprintf("%.1f%%", 100*a.composeProgress.Fraction)).FontSize(24)
		}
		ui.Button(c, "取消合成").Disabled(a.cancel == nil).OnClick(func() {
			if a.cancel != nil {
				a.cancel()
			}
		})
	})
}

func composeFilename(source, extension string, sample bool) string {
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + "-OneSub"
	if sample {
		name += "-preview"
	}
	return name + "." + extension
}

func (a *NativeApp) startCompose(sample bool) {
	if a.busy || a.snapshot == nil || a.guardChanges(func() { a.startCompose(sample) }) {
		return
	}
	state, style, mode := *a.snapshot, a.currentStyle, a.composeMode
	language := a.settings.TargetLanguage
	quality := map[string]string{"CRF 18": "original", "CRF 20": "high", "CRF 23": "standard"}[a.composeQuality]
	a.composeRunning = true
	a.composeProgress = media.ComposeProgress{Stage: "选择输出文件", Fraction: -1}
	a.evalPlayer("video.pause()")
	a.run("合成视频", func(ctx context.Context) (func(), error) {
		extension := "mp4"
		if strings.HasSuffix(mode, "MKV") {
			extension = "mkv"
		}
		output, err := a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: "合成视频", DefaultPath: composeFilename(state.Manifest.MediaPath, extension, sample), Filters: []mygo.FileFilter{{Name: strings.ToUpper(extension), Extensions: []string{extension}}}})
		if err != nil || output == "" {
			return nil, err
		}
		if !strings.EqualFold(filepath.Ext(output), "."+extension) {
			output += "." + extension
		}
		progress := func(p media.ComposeProgress) { a.win.Update(func() { a.composeProgress = p; a.status = p.Stage }) }
		progress(media.ComposeProgress{Stage: "准备字幕与字体", Fraction: -1})
		document, err := a.styledDocument(ctx, state, style)
		if err != nil {
			return nil, err
		}
		subtitleMode := "hard"
		if strings.HasPrefix(mode, "软字幕") {
			subtitleMode = "soft"
		}
		options := media.ComposeOptions{Source: state.Manifest.MediaPath, Destination: output, Mode: subtitleMode, Quality: quality, Sample: sample, Language: language, Revision: state.Revision}
		_, err = media.Compose(ctx, a.ffmpeg, a.probe, filepath.Join(a.dataRoot, "cache", "worker"), options, document, progress)
		if err != nil {
			return nil, fmt.Errorf("合成失败：%w", err)
		}
		return func() { a.lastOutput = output; a.status = "已导出 " + filepath.Base(output) }, nil
	})
}
