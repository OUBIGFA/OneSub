package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

//go:embed icons/*.svg
var iconFiles embed.FS

//go:embed branding/logo.png
var appLogo []byte

//go:embed player/index.html player/generated
var playerFiles embed.FS
var icons = map[string]*ui.SVG{}
var BuildNumber = "native-development"

// NativeApp keeps model data, not ui.Element values. The view and callbacks run
// on MyGo's main thread; background results enter through Window.Update.
type NativeApp struct {
	win                                   *mygo.Window
	player                                *mygo.WebView
	tray                                  *mygo.Tray
	store                                 *project.Store
	snapshot                              *project.Snapshot
	worker                                subtitle.Client
	preview                               *media.Server
	probe, ffmpeg, resources, dataRoot    string
	settings                              settings.Settings
	settingsOpen                          bool
	settingsTab                           int
	settingsError                         bool
	errorText, status                     string
	busy                                  bool
	choosingFile                          bool
	cancel                                context.CancelFunc
	pending                               *importing.Loaded
	candidatesOpen                        bool
	splitX, splitY, bottomSplit           float32
	list                                  ui.ListState
	selected                              int
	query                                 string
	draft                                 string
	start, end                            float64
	dirty, composing, editing             bool
	savePromptOpen                        bool
	afterSave                             func()
	position, duration                    float64
	paused                                bool
	mediaURL                              string
	playerReady                           bool
	previewPending                        bool
	previewRunning                        bool
	previewRevision                       uint64
	previewDue                            time.Time
	previewCancel                         context.CancelFunc
	observedStyle                         string
	styleSaveDue                          time.Time
	activeCue                             int
	followSuspended                       bool
	lastListFirst                         int
	modelStates                           map[string]string
	modelSelection                        string
	exportOpen                            bool
	exportFormat                          string
	quitAfterTask                         bool
	composeOpen                           bool
	composeRunning                        bool
	composeProgress                       media.ComposeProgress
	composeMode                           string
	composeQuality                        string
	lastOutput                            string
	styleClient                           styles.Client
	styleCatalog                          styles.Catalog
	currentStyle                          styles.Style
	stylePreset                           string
	subtitleTab                           int
	styleAdvanced                         bool
	openOnReady                           []string
	providerKey, keyStatus                string
	providerModels                        []string
	modelsEndpoint, modelsError           string
	modelsLoading                         bool
	fontCatalogLoading                    bool
	translationConfirm, translationReview bool
	translationCandidate                  *translationCandidate
	translationList                       ui.ListState
	translationSelected                   int
}

func iconButton(c *ui.Context, label, icon string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Children(func() { ui.Icon(c, icons[icon]); ui.Text(c, label).SingleLine() })
}
func playerButton(c *ui.Context, label, icon string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Width(36).Height(34).Children(func() { ui.Icon(c, icons[icon]) })
}

func stamp(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	ms := int64(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
func (a *NativeApp) view(c *ui.Context) {
	theme := *c.Theme()
	theme.FontSize = 15
	c.SetTheme(&theme)
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	ui.Column(c).Fill().Children(func() {
		ui.Toolbar(c, func() {
			ui.MenuButton(c, "文件", func(m *ui.Menu) {
				if m.Item("打开工程").Disabled(a.busy).Chosen() {
					a.openProject()
				}
				if m.Item("导出字幕").Disabled(a.snapshot == nil || a.busy).Chosen() {
					a.requestExport()
				}
			})
			iconButton(c, "添加文件", "folder-open").Disabled(a.busy).OnClick(a.pickFiles)
			iconButton(c, "识别字幕", "audio-lines").Disabled(a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "").OnClick(a.recognize)
			iconButton(c, "翻译字幕", "languages").Disabled(a.busy || a.snapshot == nil || len(a.snapshot.Document.Cues) == 0).OnClick(a.translate)
			ui.Spacer(c)
			ui.Button(c, "").Label("设置").Tooltip("设置").Children(func() { ui.Icon(c, icons["settings"]) }).OnClick(func() { a.settingsOpen = true })
			ui.PrimaryButton(c, "合成视频").Disabled(a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "" || len(a.snapshot.Document.Cues) == 0).OnClick(a.compose)
		}).Padding(10, 14).Background(c.Theme().Surface).Label("主操作")
		ui.Divider(c)
		ui.SplitVerticalPercent(c, &a.splitY, func() {
			ui.SplitPercent(c, &a.splitX, func() { a.playerView(c) }, func() { a.subtitlesView(c) }).Fill()
		}, func() {
			ui.SplitPercent(c, &a.bottomSplit, func() { a.editorView(c) }, func() { a.stylePanel(c) }).Fill()
		}).Grow(1)
		ui.Divider(c)
		ui.Row(c).Height(46).Padding(6, 12).Gap(12).Label("状态栏").Children(func() {
			if a.errorText != "" {
				ui.Text(c, a.errorText).TextColor(c.Theme().Danger).Grow(1).SingleLine().Tooltip(a.errorText)
			} else {
				ui.Text(c, a.status).Grow(1).SingleLine()
			}
			if a.busy && a.cancel != nil && !a.choosingFile && !a.composeRunning {
				ui.Button(c, "取消任务").OnClick(func() { a.cancel() })
			}
			if a.snapshot != nil {
				ui.Text(c, fmt.Sprintf("%d 条", len(a.snapshot.Document.Cues))).SingleLine()
			}
		})
	})
	a.settingsView(c)
	a.candidatesView(c)
	a.exportView(c)
	a.composeView(c)
	a.translationView(c)
	a.savePromptView(c)
	a.observeStyle(c.Now())
	if !a.styleSaveDue.IsZero() {
		if !c.Now().Before(a.styleSaveDue) {
			a.saveSettings()
			a.styleSaveDue = time.Time{}
		} else {
			c.After(a.styleSaveDue.Sub(c.Now()))
		}
	}
	if a.previewPending && !a.previewRunning && !a.busy && !a.dirty && a.playerReady && a.mediaURL != "" && len(a.styleCatalog.Presets) > 0 {
		if !c.Now().Before(a.previewDue) {
			a.refreshSubtitles()
		} else {
			c.After(a.previewDue.Sub(c.Now()))
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) && !a.busy {
		a.saveDraft()
	}
}
func (a *NativeApp) playerView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12).Gap(8).Label("视频播放区").Children(func() {
		ui.Row(c).Height(24).Gap(12).Children(func() {
			if a.snapshot != nil && a.snapshot.Manifest.MediaPath != "" {
				ui.Text(c, filepath.Base(a.snapshot.Manifest.MediaPath)).TextColor(c.Theme().TextMuted).SingleLine().Grow(1).TextAlign(ui.Center)
			}
		})
		if a.composeRunning {
			a.composeProgressView(c)
		} else if a.player != nil && a.mediaURL != "" {
			ui.WebView(c, a.player).Grow(1)
		} else {
			ui.Column(c).Grow(1).Center().Children(func() { iconButton(c, "添加视频", "folder-open").Disabled(a.busy).OnClick(a.pickFiles) })
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label, icon := "播放", "play"
			if !a.paused {
				label, icon = "暂停", "pause"
			}
			disabled := a.mediaURL == "" || a.composeRunning
			playerButton(c, "后退 15 秒", "rewind").Disabled(disabled).OnClick(func() { a.evalPlayer("video.currentTime=Math.max(0,video.currentTime-15)") })
			playerButton(c, label, icon).Disabled(disabled).OnClick(func() { a.evalPlayer("video.paused ? video.play() : video.pause()") })
			playerButton(c, "前进 15 秒", "fast-forward").Disabled(disabled).OnClick(func() {
				a.evalPlayer("video.currentTime=Math.min(Number.isFinite(video.duration)?video.duration:video.currentTime+15,video.currentTime+15)")
			})
			ui.Text(c, stamp(a.position)).SingleLine().MarginStart(8)
			value := a.position
			upper := math.Max(a.duration, .001)
			ui.Slider(c, &value, 0, upper).Label("播放位置").Disabled(disabled).Grow(1).OnChange(func() { a.evalPlayer(fmt.Sprintf("video.currentTime = %f", value)) })
			ui.Text(c, stamp(a.duration)).SingleLine()
		}).Label("播放控制")
	})
}
func (a *NativeApp) editorView(c *ui.Context) {
	if a.snapshot == nil || a.selected < 0 || a.selected >= len(a.snapshot.Document.Cues) {
		ui.Column(c).Fill().Padding(18, 20).Background(c.Theme().Surface).Children(func() {
			ui.Text(c, "字幕编辑").FontWeight(600)
			ui.Column(c).Grow(1).Center().Children(func() { ui.Text(c, "未选择字幕").TextColor(c.Theme().TextMuted) })
		})
		return
	}
	ui.Scroll(c).Fill().Background(c.Theme().Surface).Children(func() {
		ui.Column(c).Padding(18, 20).Gap(12).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "字幕编辑").FontWeight(600)
				ui.Spacer(c)
				if a.dirty {
					ui.Text(c, "未保存").TextColor(c.Theme().Warning)
				}
				ui.Button(c, "保存").Disabled(!a.dirty || a.busy || a.composing).OnClick(func() { a.saveDraft() })
			})
			if a.snapshot == nil || a.selected < 0 || a.selected >= len(a.snapshot.Document.Cues) {
				ui.Text(c, "未选择字幕").TextColor(c.Theme().TextMuted)
				return
			}
			cue := a.snapshot.Document.Cues[a.selected]
			readonly := cue.ReadOnly || a.busy
			ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "开始")
				ui.NumberInput(c, &a.start, 0, 9007199254740, 0.001).Width(150).Label("开始").Disabled(readonly || a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa").OnChange(func() { a.changed(c.Now()) })
				ui.Text(c, "结束")
				ui.NumberInput(c, &a.end, 0, 9007199254740, 0.001).Width(150).Label("结束").Disabled(readonly || a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa").OnChange(func() { a.changed(c.Now()) })
			})
			ui.Field(c, "字幕文字", func() {
				area := ui.TextArea(c.Key(cue.ID), &a.draft).Height(110).ReadOnly(readonly)
				a.composing = area.Composing()
				a.editing = area.Focused()
				if area.Changed() {
					a.changed(c.Now())
				}
			})
			locked := cue.TextLocked
			ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &locked, "锁定文字").Disabled(readonly).OnChange(func() { a.setTextLock(locked) })
				ui.Button(c, "播放当前句").Disabled(a.mediaURL == "").OnClick(func() { a.evalPlayer(fmt.Sprintf("video.currentTime=%f;video.play()", a.start)) })
				ui.Text(c, fmt.Sprintf("第 %d 条", a.selected+1)).TextColor(c.Theme().TextMuted)
			})
		})
	})
}
func (a *NativeApp) subtitlesView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		previousTab := a.subtitleTab
		ui.Tabs(c, &a.subtitleTab, "原文", "译文").Label("字幕语言").OnChange(func() {
			nextTab := a.subtitleTab
			a.subtitleTab = previousTab
			a.changeSubtitleTab(nextTab)
		}).Padding(8, 16)
		ui.Toolbar(c, func() {
			ui.TextInput(c, &a.query).Placeholder("搜索字幕").Label("搜索字幕").Grow(1)
			ui.Button(c, "").Label("撤销").Tooltip("撤销").Children(func() { ui.Icon(c, icons["undo-2"]) }).Disabled(a.busy || a.snapshot == nil || !a.snapshot.CanUndo).OnClick(func() { a.undo(false) })
			ui.Button(c, "").Label("重做").Tooltip("重做").Children(func() { ui.Icon(c, icons["redo-2"]) }).Disabled(a.busy || a.snapshot == nil || !a.snapshot.CanRedo).OnClick(func() { a.undo(true) })
			if a.followSuspended {
				ui.Button(c, "恢复跟随").OnClick(func() { a.followSuspended = false; a.activeCue = -1 })
			}
			if a.translationCandidate != nil {
				ui.Button(c, "审阅译文").OnClick(func() { a.translationReview = true })
			}
		}).Padding(8, 12).Label("字幕操作")
		if a.snapshot == nil || len(a.snapshot.Document.Cues) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() { ui.Text(c, "未加载字幕").TextColor(c.Theme().TextMuted) })
			return
		}
		rows := []int{}
		for i, cue := range a.snapshot.Document.Cues {
			if strings.Contains(strings.ToLower(cueDisplayText(cue, a.subtitleTab)), strings.ToLower(a.query)) {
				rows = append(rows, i)
			}
		}
		selected := -1
		for i, index := range rows {
			if index == a.selected {
				selected = i
			}
		}
		a.list.Selected = &selected
		a.list.Key = func(i int) any { return a.snapshot.Document.Cues[rows[i]].ID }
		maximum := 1
		for _, cue := range a.snapshot.Document.Cues {
			maximum = max(maximum, cue.Position+1)
		}
		cols := []ui.TableColumn{numberColumn(c, maximum), {Title: "开始", Width: 128}, {Title: "结束", Width: 128}, {Title: "字幕"}}
		table := ui.Table(c, &a.list, cols, len(rows), func(row, col int) {
			cue := a.snapshot.Document.Cues[rows[row]]
			value := cueDisplayText(cue, a.subtitleTab)
			switch col {
			case 0:
				value = fmt.Sprint(cue.Position + 1)
			case 1:
				value = stamp(float64(cue.StartMs) / 1000)
			case 2:
				value = stamp(float64(cue.EndMs) / 1000)
			}
			ui.Text(c, value).SingleLine()
		}).Grow(1).Label("字幕列表")
		if (table.Changed() || table.Submitted()) && selected >= 0 && selected < len(rows) {
			action := a.settings.SingleClickAction
			if table.Submitted() {
				action = a.settings.DoubleClickAction
			}
			a.selectSubtitle(rows[selected], action)
		}
		// Follow playback only outside text edits. A manually scrolled-away row
		// pauses following until the user explicitly resumes it.
		first, last := a.list.Visible()
		if first != a.lastListFirst && a.activeCue >= 0 && selected >= 0 && (selected < first || selected > last) {
			a.followSuspended = true
		}
		a.lastListFirst = first
		if a.settings.FollowPlayback && !a.followSuspended && !a.paused && !a.dirty && !a.composing && !a.editing && !a.savePromptOpen {
			active := cueAt(a.snapshot.Document.Cues, a.position)
			if active >= 0 && active != a.activeCue {
				a.activeCue = active
				for row, index := range rows {
					if index == active {
						a.selected = index
						a.loadDraft()
						a.list.ScrollIntoView(row)
						break
					}
				}
			}
		}
	})
}
func (a *NativeApp) changed(_ time.Time) {
	if a.snapshot == nil || a.selected < 0 {
		return
	}
	cue := a.snapshot.Document.Cues[a.selected]
	a.dirty = a.draft != cueDisplayText(cue, a.subtitleTab) || int64(math.Round(a.start*1000)) != cue.StartMs || int64(math.Round(a.end*1000)) != cue.EndMs
	a.status = "已保存"
	if a.dirty {
		a.status = "未保存"
	}
}
func (a *NativeApp) loadDraft() {
	if a.snapshot != nil && a.selected >= 0 && a.selected < len(a.snapshot.Document.Cues) {
		cue := a.snapshot.Document.Cues[a.selected]
		a.draft = cueDisplayText(cue, a.subtitleTab)
		a.start = float64(cue.StartMs) / 1000
		a.end = float64(cue.EndMs) / 1000
	} else {
		a.draft = ""
		a.start = 0
		a.end = 0
	}
	a.dirty = false
}
func (a *NativeApp) saveDraft() bool {
	if !a.dirty {
		return true
	}
	if a.composing {
		a.errorText = "请先完成中文输入"
		return false
	}
	if a.store == nil || a.snapshot == nil {
		return false
	}
	cue := a.snapshot.Document.Cues[a.selected]
	track := "source"
	if a.subtitleTab == 1 {
		track = "translation"
	}
	_, err := a.store.Apply(uuid.NewString(), a.snapshot.Revision, []project.Edit{{ID: cue.ID, Text: a.draft, Track: track, StartMs: int64(math.Round(a.start * 1000)), EndMs: int64(math.Round(a.end * 1000))}})
	if err != nil {
		a.errorText = err.Error()
		a.status = "保存失败"
		return false
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return false
	}
	a.snapshot = &state
	a.previewPending = true
	a.dirty = false
	a.errorText = ""
	a.status = "已保存"
	return true
}
func (a *NativeApp) undo(redo bool) {
	if a.store == nil || a.guardChanges(func() { a.undo(redo) }) {
		return
	}
	if _, err := a.store.Undo(uuid.NewString(), a.snapshot.Revision, redo); err != nil {
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
	if a.subtitleTab == 1 {
		hasTranslation := false
		for _, cue := range state.Document.Cues {
			if cue.Translated {
				hasTranslation = true
				break
			}
		}
		if !hasTranslation {
			a.subtitleTab = 0
		}
	}
	a.loadDraft()
	a.status = "已保存"
}

// run owns one cancellable operation. Closing to tray does not end this context.
func (a *NativeApp) run(label string, work func(context.Context) (func(), error)) {
	if a.busy || a.guardChanges(func() { a.run(label, work) }) {
		return
	}
	a.busy = true
	a.errorText = ""
	a.status = label
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go func() {
		apply, err := work(ctx)
		cancel()
		a.win.Update(func() {
			a.busy = false
			a.composeRunning = false
			a.cancel = nil
			if err != nil {
				a.errorText = err.Error()
				a.status = "操作未完成"
			} else {
				if apply != nil {
					a.status = "已完成"
					apply()
				} else {
					a.status = ""
				}
			}
			if a.quitAfterTask {
				a.quitAfterTask = false
				a.requestExit()
			}
		})
	}()
}
func (a *NativeApp) pickFiles() {
	a.run("打开文件", func(ctx context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: "添加文件", Multiple: true, Filters: []mygo.FileFilter{{Name: "视频、音频与字幕", Extensions: importing.Extensions}}})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return a.importFiles(ctx, paths)
	})
}
func (a *NativeApp) importFiles(ctx context.Context, paths []string) (func(), error) {
	selection, err := importing.Discover(paths)
	if err != nil {
		return nil, err
	}
	loaded, err := importing.Load(ctx, selection, a.worker, a.probe)
	if err != nil {
		return nil, err
	}
	name := selection.Subtitle
	if name == "" {
		name = selection.Media
	}
	name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	root := filepath.Join(a.dataRoot, "projects")
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	next, err := project.Create(filepath.Join(root, name+"-"+uuid.NewString()+".onesub"), loaded.Selection.Media, loaded.Selection.Subtitle, loaded.Document, name)
	if err != nil {
		return nil, err
	}
	state, err := next.Snapshot()
	if err != nil {
		next.Close()
		return nil, err
	}
	url, previewErr := a.preview.Register(loaded.Selection.Media)
	return func() {
		if a.store != nil {
			a.store.Close()
		}
		a.store = next
		a.snapshot = &state
		a.subtitleTab = 0
		a.selected = 0
		a.loadDraft()
		a.pending = &loaded
		a.candidatesOpen = len(selection.Candidates) > 0
		a.mediaURL = url
		a.position = 0
		a.activeCue = -1
		a.followSuspended = false
		a.duration = float64(loaded.Media.DurationMs()) / 1000
		a.paused = true
		a.loadVideo()
		a.settings.RecentProjects = append([]string{state.Path}, a.settings.RecentProjects...)
		if len(a.settings.RecentProjects) > 20 {
			a.settings.RecentProjects = a.settings.RecentProjects[:20]
		}
		a.saveSettings()
		a.errorText = strings.Join(loaded.Warnings, "；")
		if previewErr != nil {
			a.errorText = previewErr.Error()
		}
	}, nil
}
func (a *NativeApp) openProject() {
	a.run("打开工程", func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: "打开工程", Directory: true})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		next, err := project.Open(paths[0])
		if err != nil {
			return nil, err
		}
		state, err := next.Snapshot()
		if err != nil {
			next.Close()
			return nil, err
		}
		url, previewErr := a.preview.Register(state.Manifest.MediaPath)
		return func() {
			if a.store != nil {
				a.store.Close()
			}
			a.store = next
			a.snapshot = &state
			a.subtitleTab = 0
			for _, cue := range state.Document.Cues {
				if cue.Translated {
					a.subtitleTab = 1
					break
				}
			}
			a.selected = 0
			a.loadDraft()
			a.mediaURL = url
			a.loadVideo()
			if previewErr != nil {
				a.errorText = "视频已移动，请重新添加视频"
			}
		}, nil
	})
}
func (a *NativeApp) candidatesView(c *ui.Context) {
	ui.Modal(c, &a.candidatesOpen, func() {
		ui.Text(c, "选择关联文件")
		if a.pending != nil {
			for _, candidate := range a.pending.Selection.Candidates {
				path := candidate
				ui.Button(c, filepath.Base(path)).OnClick(func() {
					a.candidatesOpen = false
					paths := []string{path}
					if a.pending.Selection.CandidateKind == "subtitle" {
						paths = append(paths, a.pending.Selection.Media)
					} else {
						paths = append(paths, a.pending.Selection.Subtitle)
					}
					a.run("关联文件", func(ctx context.Context) (func(), error) { return a.importFiles(ctx, paths) })
				})
			}
		}
		ui.Button(c, "稍后选择").OnClick(func() { a.candidatesOpen = false })
	})
}
func (a *NativeApp) exportView(c *ui.Context) {
	ui.Modal(c, &a.exportOpen, func() {
		ui.Text(c, "导出字幕")
		formats := []string{"srt", "vtt"}
		if a.snapshot != nil && (a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa") {
			formats = append(formats, a.snapshot.Document.Format)
		}
		ui.Select(c, &a.exportFormat, formats).Label("格式")
		ui.Button(c, "导出").OnClick(func() { a.exportOpen = false; a.exportSubtitle() })
	})
}
func (a *NativeApp) exportSubtitle() {
	if a.snapshot == nil || a.guardChanges(a.exportSubtitle) {
		return
	}
	state := *a.snapshot
	format := a.exportFormat
	a.run("导出字幕", func(ctx context.Context) (func(), error) {
		path, err := a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: "导出字幕", DefaultPath: state.Manifest.Name + ".edited." + format, Filters: []mygo.FileFilter{{Name: strings.ToUpper(format), Extensions: []string{format}}}})
		if err != nil || path == "" {
			return nil, err
		}
		if !strings.EqualFold(filepath.Ext(path), "."+format) {
			path += "." + format
		}
		state.Document.TargetFormat = format
		data, err := a.worker.Write(ctx, state.Document)
		if err != nil {
			return nil, err
		}
		err = subtitle.Publish(path, data)
		return func() { a.status = "已导出 " + filepath.Base(path) }, err
	})
}

func (a *NativeApp) saveSettings() {
	if a.settingsError {
		a.errorText = "设置文件需要修复，未覆盖原文件"
		return
	}
	if err := settings.Save(filepath.Join(a.dataRoot, "settings.json"), a.settings); err != nil {
		a.errorText = err.Error()
	}
}
func (a *NativeApp) settingsView(c *ui.Context) {
	width, height := c.Size()
	ui.Modal(c, &a.settingsOpen, func() {
		ui.Text(c, "设置").FontSize(18).FontWeight(600)
		ui.Tabs(c, &a.settingsTab, "常规", "识别", "模型", "翻译").Label("设置分类")
		// The normal layout fits without scrolling. Keep a height cap only for
		// small displays and enlarged accessibility text instead of clipping fields.
		ui.Scroll(c).Height(max(180, min(440, height-240))).Width(min(820, width-96)).Label("设置内容").Children(func() {
			ui.Column(c).Gap(16).Padding(4, 0).Children(func() { a.settingsContent(c) })
		})
		ui.Row(c).Justify(ui.End).Children(func() { ui.Button(c, "关闭").OnClick(func() { a.saveSettings(); a.settingsOpen = false }) })
	})
}
func (a *NativeApp) modelChoice(c *ui.Context, label, kind string, id *string) {
	labels := []string{}
	selected := ""
	for _, m := range models.Catalog() {
		if m.Kind == kind {
			labels = append(labels, m.Label)
			if *id == m.ID {
				selected = m.Label
			}
		}
	}
	ui.Field(c, label, func() {
		ui.Select(c, &selected, labels).OnChange(func() {
			for _, m := range models.Catalog() {
				if m.Label == selected {
					*id = m.ID
					break
				}
			}
			a.saveSettings()
		})
	})
}
func (a *NativeApp) modelPath(id string) string {
	if path := a.settings.ModelPaths[id]; path != "" {
		return path
	}
	return filepath.Join(a.dataRoot, "models", id)
}
func (a *NativeApp) pickModelDir(m models.Model) {
	a.run("选择模型目录", func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: m.Label, Directory: true, DefaultPath: a.modelPath(m.ID)})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() { a.settings.ModelPaths[m.ID] = paths[0]; a.modelStates[m.ID] = "待校验"; a.saveSettings() }, nil
	})
}
func (a *NativeApp) verifyModel(m models.Model) {
	dir := a.modelPath(m.ID)
	a.run("校验模型", func(ctx context.Context) (func(), error) {
		err := models.Verify(ctx, m, dir)
		return func() { a.modelStates[m.ID] = "已校验" }, err
	})
}
func (a *NativeApp) downloadModel(m models.Model) {
	dir := a.modelPath(m.ID)
	a.run("下载模型", func(ctx context.Context) (func(), error) {
		err := models.Download(ctx, &http.Client{Timeout: 2 * time.Hour}, m, dir, func(p models.Progress) {
			a.win.Update(func() { a.modelStates[m.ID] = fmt.Sprintf("%s %.1f%%", p.Stage, float64(p.Done)*100/float64(p.Total)) })
		})
		return func() { a.modelStates[m.ID] = "已校验" }, err
	})
}

func (a *NativeApp) evalPlayer(script string) {
	if a.player == nil {
		return
	}
	go func() {
		_, err := a.player.Page().Eval(script)
		if err != nil {
			a.win.Update(func() { a.errorText = "播放操作失败：" + err.Error() })
		}
	}()
}
func (a *NativeApp) loadVideo() {
	if !a.playerReady {
		return
	}
	a.previewPending = true
	if a.mediaURL == "" {
		a.evalPlayer("video.pause();video.removeAttribute('src');video.load();setSubtitles(null);window.previewError=''")
		return
	}
	value, _ := json.Marshal(a.mediaURL)
	a.evalPlayer("video.pause();setSubtitles(null);window.previewError='';video.src=" + string(value) + ";video.load()")
}
func (a *NativeApp) requestExit() {
	if a.busy {
		go func() {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Title: "退出 OneSub", Message: "任务仍在运行", Buttons: []string{"等待完成后退出", "取消任务并退出", "返回"}, CancelButton: 2})
			if err != nil {
				return
			}
			a.win.Update(func() {
				switch result.Button {
				case 0:
					a.quitAfterTask = true
				case 1:
					a.quitAfterTask = true
					if a.cancel != nil {
						a.cancel()
					}
				}
			})
		}()
		return
	}
	if a.guardChanges(a.requestExit) {
		return
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			a.errorText = err.Error()
			return
		}
		a.store = nil
	}
	a.saveSettings()
	if a.previewCancel != nil {
		a.previewCancel()
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
	a.win.Destroy()
}
func (a *NativeApp) makeTray() error {
	image, err := trayLogo(mygo.Screen.PrimaryDisplay().ScaleFactor)
	if err != nil {
		return err
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "onesub-main", Icon: image, ToolTip: "OneSub", Menu: mygo.NewMenu([]*mygo.MenuItem{{Label: "打开 OneSub", Click: func(*mygo.MenuItem, *mygo.Window) { a.activate() }}, mygo.Separator(), {Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { a.requestExit() }}})})
	a.tray = tray
	return err
}

// activate is shared by second launches and the tray, including minimized windows.
func (a *NativeApp) activate() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
}

func main() {
	// Claim the application before any worker, data store or tray is created.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	playerFS, err := fs.Sub(playerFiles, "player")
	if err != nil {
		log.Fatal(err)
	}
	mygo.SetFrontend(playerFS)
	names, err := iconFiles.ReadDir("icons")
	if err != nil {
		log.Fatal(err)
	}
	for _, name := range names {
		raw, err := iconFiles.ReadFile("icons/" + name.Name())
		if err != nil {
			log.Fatal(err)
		}
		icons[strings.TrimSuffix(name.Name(), ".svg")] = ui.MustParseSVG(raw)
	}
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		log.Fatal(err)
	}
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal(err)
	}
	if override := os.Getenv("ONESUB_DATA_ROOT"); override != "" {
		data = override
		mygo.App.SetPath(mygo.PathUserData, data)
	}
	scratch := filepath.Join(data, "cache", "worker")
	if err = os.MkdirAll(scratch, 0700); err != nil {
		log.Fatal(err)
	}
	prefs, prefsErr := settings.Load(filepath.Join(data, "settings.json"))
	preview, err := media.NewServer()
	if err != nil {
		log.Fatal(err)
	}
	defer preview.Close()
	a := &NativeApp{resources: resources, dataRoot: data, settings: prefs, settingsError: prefsErr != nil, preview: preview, worker: subtitle.Client{Executable: filepath.Join(resources, "subtitle-core", "OneSub.SubtitleCore.exe"), Scratch: scratch}, probe: filepath.Join(resources, "media-tools", "ffprobe.exe"), ffmpeg: filepath.Join(resources, "media-tools", "ffmpeg.exe"), splitX: 60, splitY: 64, bottomSplit: 42, selected: -1, paused: true, modelStates: map[string]string{}, exportFormat: "srt"}
	if prefsErr != nil {
		a.errorText = "设置读取失败，原文件已保留"
	}
	if configured := os.Getenv("ONESUB_SUBTITLE_WORKER"); configured != "" {
		a.worker.Executable = configured
	}
	if configured := os.Getenv("ONESUB_FFPROBE"); configured != "" {
		a.probe = configured
	}
	if configured := os.Getenv("ONESUB_FFMPEG"); configured != "" {
		a.ffmpeg = configured
	}
	// Accept explicit launch paths without registering OneSub as the system's
	// default video player. MyGo dispatches OS associations separately.
	for _, argument := range os.Args[1:] {
		if importing.Kind(argument) != "" {
			a.openOnReady = append(a.openOnReady, argument)
		}
	}
	mygo.App.OnSecondInstance(func([]string, string) { a.activate() })
	mygo.App.OnOpenFile(func(path string) {
		if a.win == nil {
			a.openOnReady = append(a.openOnReady, path)
			return
		}
		a.run("导入文件", func(ctx context.Context) (func(), error) { return a.importFiles(ctx, []string{path}) })
	})
	mygo.App.WhenReady(func() {
		display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
		bounds := initialWindowBounds(display.WorkArea)
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "OneSub", X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height, MinWidth: min(900, bounds.Width), MinHeight: min(640, bounds.Height), StateKey: "main", Vibrancy: mygo.VibrancyMica, Content: ui.View(a.view)})
		a.player, err = a.win.NewWebView(mygo.WebViewOptions{URL: "/"})
		if err != nil {
			a.errorText = err.Error()
		} else {
			a.player.Page().OnDOMReady(func() { a.win.Update(func() { a.playerReady = true; a.loadVideo() }) })
		}
		if err = a.makeTray(); err != nil {
			a.errorText = "托盘不可用，将保留窗口"
		}
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			paths := append([]string{}, e.Paths...)
			a.run("导入文件", func(ctx context.Context) (func(), error) { return a.importFiles(ctx, paths) })
		})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			e.PreventDefault()
			if a.settings.CloseToTray && a.tray != nil {
				a.hideToTray()
			} else {
				a.requestExit()
			}
		})
		a.styleClient = styles.Client{Executable: filepath.Join(resources, "style-core", "OneSub.StyleCore.exe"), Scratch: scratch}
		if value := os.Getenv("ONESUB_STYLE_WORKER"); value != "" {
			a.styleClient.Executable = value
		}
		a.reloadFonts()
		fontContext, stopFonts := context.WithCancel(context.Background())
		mygo.App.OnQuit(stopFonts)
		go styles.WatchFonts(fontContext, func() { a.win.Update(a.reloadFonts) })
		if len(a.openOnReady) > 0 {
			paths := a.openOnReady
			a.openOnReady = nil
			a.run("导入文件", func(ctx context.Context) (func(), error) { return a.importFiles(ctx, paths) })
		}
		go func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				if a.player == nil {
					continue
				}
				result, err := mygo.EvalAs[struct {
					Position float64 `json:"position"`
					Duration float64 `json:"duration"`
					Paused   bool    `json:"paused"`
					Error    string  `json:"error"`
				}](a.player.Page(), "({position:video.currentTime,duration:Number.isFinite(video.duration)?video.duration:0,paused:video.paused,error:window.previewError||''})")
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					continue
				}
				a.win.Update(func() {
					a.position = result.Position
					a.duration = result.Duration
					a.paused = result.Paused
					if result.Error != "" {
						a.errorText = result.Error
					}
				})
			}
		}()
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
