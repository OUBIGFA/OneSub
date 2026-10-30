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
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
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
	win                                  *mygo.Window
	player                               *mygo.WebView
	tray                                 *mygo.Tray
	store                                *project.Store
	snapshot                             *project.Snapshot
	worker                               subtitle.Client
	preview                              *media.Server
	probe, ffmpeg, resources, dataRoot   string
	settings                             settings.Settings
	settingsOpen                         bool
	settingsTab                          int
	settingsError                        bool
	errorText, status                    string
	errorShown                           string    // the error the status bar shows, since errorSince
	errorSince                           time.Time // when it began to show
	playerError                          string    // the page's last error code
	rate                                 float64   // the playback speed; 0 is normal
	loop                                 bool      // the video starts again at its end
	busy                                 bool
	choosingFile                         bool
	cancel                               context.CancelFunc
	pending                              *importing.Loaded
	candidatesOpen                       bool
	splitX, splitY, bottomSplit          float32
	styleSplit                           float32 // the style's share of the room right of the editor
	workflowTab                          int     // 0 recognition, 1 translation
	list                                 ui.ListState
	selected                             int
	query                                string
	draft                                string
	start, end                           float64
	editPending, composing, editing      bool
	editDue                              time.Time
	savedHash, savedPath                 [2]string
	unsaved                              [2]bool
	savePromptOpen, clearOpen, skipGuard bool
	afterSave                            func()
	position, duration                   float64
	paused                               bool
	mediaURL                             string
	playerReady                          bool
	previewPending                       bool
	previewRunning                       bool
	previewRevision                      uint64
	previewDue                           time.Time
	previewCancel                        context.CancelFunc
	observedStyle                        string
	prefsSaveDue                         time.Time
	activeCue                            int
	followPaused                         bool    // the user scrolled the list away from the playhead
	followRow, locateCue                 int     // the row last centered; a cue to center, or -1
	frameAspect                          float64 // the video's display width / height
	modelStates                          map[string]string
	modelSelection                       string
	modelOpen                            bool
	exportOpen                           bool
	exportFormat                         string
	quitAfterTask                        bool
	composeOpen                          bool
	composeRunning                       bool
	recognitionRunning                   bool
	recognitionStarted                   time.Time
	recognitionStep                      recognitionStep
	translationRunning                   bool
	translationLive                      *translationLive // what the running translation has shown
	translationTask                      *translationTask
	composeProgress                      media.ComposeProgress
	lastOutput                           string
	styleClient                          styles.Client
	styleCatalog                         styles.Catalog
	currentStyle                         styles.Style
	secondaryStyle                       styles.Style
	secondaryPreset                      string
	styleLayer                           int
	stylePreset                          string
	presetDialog                         string // "", "save", "rename" or "delete"
	presetDialogLayer                    int
	presetName                           string
	styleWidths                          [3]float32 // the style sections' widths in the last frame
	styleRoom                            float32    // the width the style sections have
	subtitleTab                          int
	editRebuiltSource                    bool
	openOnReady                          []string
	providerKey, keyStatus               string
	providerModels                       []string
	modelsEndpoint, modelsError          string
	modelsLoading                        bool
	modelsGeneration                     uint64
	providerDelete                       bool
	fontCatalogLoading                   bool
	translationReview                    bool // the check results of the last translation are shown
	translationCandidate                 *translationCandidate
	translationList                      ui.ListState
	translationProfiles                  []translator.Profile
	profilesError                        string
	translationSelected                  int
}

func stamp(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	ms := int64(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
func (a *NativeApp) view(c *ui.Context) {
	theme, look := studioTheme(c.Theme(), c.Preferences().HighContrast)
	c.SetTheme(&theme)
	a.keepPaneMinimums(c)
	ui.Column(c).Fill().Children(func() {
		a.topBar(c, look)
		ui.Divider(c)
		ui.SplitVerticalPercent(c, &a.splitY, func() {
			ui.SplitPercent(c, &a.splitX, func() { a.playerView(c) }, func() { a.subtitlesView(c) }).Fill()
		}, func() {
			ui.SplitPercent(c, &a.bottomSplit, func() { a.editorView(c) }, func() {
				ui.SplitPercent(c, &a.styleSplit, func() { a.stylePanel(c) }, func() { a.workflowPanel(c) }).Fill()
			}).Fill()
		}).Grow(1)
		ui.Divider(c)
		a.statusBar(c, look)
	})
	a.settingsView(c)
	a.modelManagerView(c)
	a.candidatesView(c)
	a.exportView(c)
	a.composeView(c)
	a.translationView(c)
	a.providerDeleteView(c)
	a.savePromptView(c)
	a.clearView(c)
	a.presetDialogView(c)
	a.observeStyle(c.Now())
	// The panes keep the sizes the user drags them to, saved once the drag
	// stops, as the other preferences.
	if w, h, e, s := float64(a.splitX), float64(a.splitY), float64(a.bottomSplit), float64(a.styleSplit); w != a.settings.PlayerWidth || h != a.settings.TopHeight || e != a.settings.EditorWidth || s != a.settings.StyleWidth {
		a.settings.PlayerWidth, a.settings.TopHeight, a.settings.EditorWidth, a.settings.StyleWidth = w, h, e, s
		a.prefsSaveDue = c.Now().Add(400 * time.Millisecond)
	}
	if !a.prefsSaveDue.IsZero() {
		if !c.Now().Before(a.prefsSaveDue) {
			a.saveSettings()
			a.prefsSaveDue = time.Time{}
		} else {
			c.After(a.prefsSaveDue.Sub(c.Now()))
		}
	}
	// Edits reach the project after a pause in typing, never in the middle of
	// an input-method composition.
	if a.editPending && !a.composing && !a.editDue.IsZero() {
		if !c.Now().Before(a.editDue) {
			a.commitEdit(false)
		} else {
			c.After(a.editDue.Sub(c.Now()))
		}
	}
	if a.previewPending && !a.previewRunning && !a.busy && !a.editPending && a.playerReady && a.mediaURL != "" && len(a.styleCatalog.Presets) > 0 {
		if !c.Now().Before(a.previewDue) {
			a.refreshSubtitles()
		} else {
			c.After(a.previewDue.Sub(c.Now()))
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) && !a.busy {
		a.saveTrack(a.subtitleTab, false)
	}
}

// keepPaneMinimums keeps each pane the room its controls need, in both
// languages and at the smallest window: a divider dragged further stops
// there, before the panes are laid out.
func (a *NativeApp) keepPaneMinimums(c *ui.Context) {
	width, height := c.Size()
	height -= em(c, 44+32) // the tool bar and the status bar
	keep := func(percent *float32, room, first, second float32) {
		if room >= first+second {
			*percent = min(max(*percent, first/room*100), 100-second/room*100)
		}
	}
	keep(&a.splitX, width, em(c, 500), em(c, 360))
	keep(&a.splitY, height, em(c, 320), em(c, 180))
	keep(&a.bottomSplit, width, em(c, 260), em(c, 340+300))
	keep(&a.styleSplit, width*(100-a.bottomSplit)/100, em(c, 340), em(c, 300))
}

// topBar holds the actions on the whole workbench: adding and clearing on
// the left; settings and the two outputs on the right, video export last as
// the one primary action, where editors put Export.
func (a *NativeApp) topBar(c *ui.Context, look studio) {
	ui.Toolbar(c, func() {
		iconButton(c, locales.ActionAddFiles(), "folder-open").Disabled(a.busy).OnClick(a.pickFiles)
		iconButton(c, locales.ActionClearWorkbench(), "eraser").Disabled(a.busy || a.snapshot == nil).OnClick(a.requestClear)
		ui.Spacer(c)
		toolIcon(c, locales.ActionSettings(), "settings").OnClick(func() { a.reloadTranslationProfiles(); a.settingsOpen = true })
		iconButton(c, locales.MenuExportSubtitles(), "file-down").Disabled(a.busy || a.snapshot == nil || len(a.snapshot.Document.Cues) == 0).OnClick(a.requestExport)
		primaryIconButton(c, locales.ActionCompose(), "clapperboard").Disabled(a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "" || len(a.snapshot.Document.Cues) == 0).OnClick(a.compose)
	}).Gap(em(c, 8)).Padding(em(c, 6), em(c, 12)).Background(look.chrome).Label(locales.A11yToolbar())
}

// errorDuration is how long the status bar shows an error before it goes
// back to the last status.
const errorDuration = 8 * time.Second

// statusBar reports the last result in caption text. An error shows in the
// danger color, with its full text in the tooltip, for errorDuration.
func (a *NativeApp) statusBar(c *ui.Context, look studio) {
	if a.errorText != a.errorShown {
		a.errorShown, a.errorSince = a.errorText, c.Now()
	}
	if a.errorText != "" {
		if left := errorDuration - c.Now().Sub(a.errorSince); left > 0 {
			c.After(left)
		} else {
			a.errorText, a.errorShown = "", ""
		}
	}
	ui.Row(c).Height(em(c, 32)).Padding(0, em(c, 12)).Gap(em(c, 12)).AlignItems(ui.Center).Background(look.chrome).FontSize(em(c, captionSize)).Label(locales.A11yStatusBar()).Children(func() {
		if a.errorText != "" {
			ui.Text(c, a.errorText).TextColor(c.Theme().Danger).Grow(1).SingleLine().Tooltip(a.errorText)
		} else {
			muted(c, a.status).Grow(1).SingleLine()
		}
		if a.busy && a.cancel != nil && !a.choosingFile && !a.composeRunning && !a.recognitionRunning && !a.translationRunning {
			ui.Button(c, locales.ActionCancelTask()).Padding(em(c, 2), em(c, 10)).OnClick(func() { a.cancel() })
		}
		if a.lastOutput != "" && !a.busy {
			ui.Button(c, locales.ResultOpenFolder()).Padding(em(c, 2), em(c, 10)).OnClick(func() {
				if err := mygo.Shell.OpenPath(filepath.Dir(a.lastOutput)); err != nil {
					a.errorText = err.Error()
				}
			})
		}
		if a.snapshot != nil {
			muted(c, locales.StatusCueCount(len(a.snapshot.Document.Cues))).SingleLine()
		}
	})
}

// run owns one cancellable operation. Closing to tray does not end this context.
// Callers that replace the workbench ask about unsaved subtitle files first.
func (a *NativeApp) run(label string, work func(context.Context) (func(), error)) {
	if a.busy {
		return
	}
	a.commitEdit(true)
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
			a.recognitionRunning = false
			if a.translationRunning && err != nil && a.translationLive != nil {
				a.translationLive.stopped = err.Error()
				if errors.Is(err, context.Canceled) {
					a.translationLive.stopped = locales.StatusCancelled()
				}
			}
			a.translationRunning = false
			a.cancel = nil
			switch {
			case errors.Is(err, context.Canceled):
				// Cancelling is the user's choice, not a failure.
				a.status = locales.StatusCancelled()
			case err != nil:
				a.errorText = err.Error()
				a.status = locales.StatusFailed()
			case apply != nil:
				a.status = locales.StatusDone()
				apply()
			default:
				a.status = ""
			}
			if a.quitAfterTask {
				a.quitAfterTask = false
				a.requestExit()
			}
		})
	}()
}
func (a *NativeApp) pickFiles() {
	if a.guardUnsaved(a.pickFiles) {
		return
	}
	a.run(locales.TaskOpenFiles(), func(ctx context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.ActionAddFiles(), Multiple: true, Filters: []mygo.FileFilter{{Name: locales.FilterMediaAndSubtitles(), Extensions: importing.Extensions}}})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return a.importFiles(ctx, paths)
	})
}

// importPaths is the shared entry of drops, launches and associations.
func (a *NativeApp) importPaths(paths []string) {
	if a.guardUnsaved(func() { a.importPaths(paths) }) {
		return
	}
	a.run(locales.TaskImport(), func(ctx context.Context) (func(), error) { return a.importFiles(ctx, paths) })
}
func (a *NativeApp) importFiles(ctx context.Context, paths []string, wordEvidence ...[]byte) (func(), error) {
	selection, err := importing.Discover(paths)
	if err != nil {
		return nil, err
	}
	loaded, err := importing.Load(ctx, selection, a.worker, a.probe)
	if err != nil {
		return nil, err
	}
	if len(wordEvidence) > 0 {
		// Faulty word timings cost only finer splits later, never the
		// recognized subtitles: they are left out, with a notice.
		if attached, wordWarnings, attachErr := subtitle.AttachOneAsrWords(loaded.Document, wordEvidence[0]); attachErr != nil {
			loaded.Warnings = append(loaded.Warnings, locales.WarnWordsSkipped(attachErr.Error()))
		} else {
			loaded.Document = attached
			loaded.Warnings = append(loaded.Warnings, wordWarnings...)
		}
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
	if len(wordEvidence) > 0 {
		// Retain the unmodified aligner artifact independently of the safe
		// split anchors attached to cues; zero-width tokens are not discarded.
		if err = os.WriteFile(filepath.Join(state.Path, "originals", "oneasr-words.json"), wordEvidence[0], 0600); err != nil {
			next.Close()
			return nil, err
		}
	}
	url, previewErr := a.preview.Register(loaded.Selection.Media)
	return func() {
		a.replaceProject(next, state, url)
		a.pending = &loaded
		a.candidatesOpen = len(selection.Candidates) > 0
		a.duration = float64(loaded.Media.DurationMs()) / 1000
		a.frameAspect = loaded.Media.FrameAspect()
		a.errorText = strings.Join(loaded.Warnings, locales.ListSeparator())
		if previewErr != nil {
			a.errorText = previewErr.Error()
		}
	}, nil
}

// replaceProject switches the workbench to another project. The subtitle
// file it came from is the saved baseline of the source track.
func (a *NativeApp) replaceProject(next *project.Store, state project.Snapshot, url string) {
	if a.store != nil {
		a.store.Close()
	}
	a.store = next
	a.subtitleTab = 0
	a.selected = 0
	a.translationCandidate = nil
	a.translationLive, a.translationTask = nil, nil
	a.settings.TranslationRecovery = ""
	a.saveSettings()
	a.setSnapshot(state)
	a.markSaved(0, a.originalSubtitlePath())
	a.markSaved(1, "")
	a.loadDraft()
	a.mediaURL = url
	a.position = 0
	a.activeCue = -1
	a.paused = true
	a.loadVideo()
}
func (a *NativeApp) candidatesView(c *ui.Context) {
	ui.Modal(c, &a.candidatesOpen, func() {
		dialogTitle(c, locales.CandidatesTitle())
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
					a.importPaths(paths)
				})
			}
		}
		ui.Button(c, locales.CandidatesLater()).OnClick(func() { a.candidatesOpen = false })
	})
}
func (a *NativeApp) saveSettings() {
	a.settings.SyncProvider()
	if a.settingsError {
		a.errorText = locales.ErrSettingsNeedRepair()
		return
	}
	if err := settings.Save(filepath.Join(a.dataRoot, "settings.json"), a.settings); err != nil {
		a.errorText = err.Error()
	}
}
func (a *NativeApp) settingsView(c *ui.Context) {
	width, height := c.Size()
	ui.Modal(c, &a.settingsOpen, func() {
		dialogTitle(c, locales.SettingsTitle())
		ui.Tabs(c, &a.settingsTab, locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsTranslation()).Label(locales.A11ySettingsTabs())
		// The pages keep one size; only a window too short for them scrolls
		// them, rather than cut them or the Close button off.
		ui.Scroll(c).MaxHeight(height - em(c, 230)).Children(func() {
			// Overlapping native grid cells reserve the tallest page's intrinsic
			// height. Invisible pages retain layout, but not input or accessibility.
			ui.Grid(c).Columns(1).Width(min(em(c, 940), width-96)).Label(locales.A11ySettingsContent()).Children(func() {
				for tab := 0; tab < 3; tab++ {
					page := ui.Column(c.Key(tab)).RowStart(1).ColumnStart(1).Gap(em(c, 28)).Padding(em(c, 12), 0).AlignSelf(ui.Start)
					if tab != a.settingsTab {
						page.Invisible()
					}
					page.Children(func() { a.settingsContent(c, tab) })
				}
			})
		})
		ui.Row(c).Justify(ui.End).Children(func() {
			ui.Button(c, locales.ActionClose()).OnClick(func() { a.saveSettings(); a.settingsOpen = false })
		})
	})
}
func (a *NativeApp) modelPath(id string) string { return a.toolset().modelPath(a.settings, id) }
func (a *NativeApp) pickModelDir(m models.Model) {
	a.run(locales.TaskChooseModelFolder(), func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: m.Label, Directory: true, DefaultPath: a.modelPath(m.ID)})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() {
			a.settings.ModelPaths[m.ID] = paths[0]
			a.modelStates[m.ID] = locales.ModelStateUnverified()
			a.saveSettings()
		}, nil
	})
}
func (a *NativeApp) verifyModel(m models.Model) {
	dir := a.modelPath(m.ID)
	a.run(locales.TaskVerifyModel(), func(ctx context.Context) (func(), error) {
		err := models.Verify(ctx, m, dir)
		return func() { a.modelStates[m.ID] = locales.ModelStateVerified() }, err
	})
}
func (a *NativeApp) downloadModel(m models.Model) {
	dir := a.modelPath(m.ID)
	a.run(locales.TaskDownloadModel(), func(ctx context.Context) (func(), error) {
		err := models.Download(ctx, &http.Client{Timeout: 2 * time.Hour}, m, dir, func(p models.Progress) {
			a.win.Update(func() {
				a.modelStates[m.ID] = locales.ModelProgress(modelStageLabel(p.Stage), float64(p.Done)*100/float64(p.Total))
			})
		})
		return func() { a.modelStates[m.ID] = locales.ModelStateVerified() }, err
	})
}

func (a *NativeApp) evalPlayer(script string) {
	if a.player == nil {
		return
	}
	go func() {
		_, err := a.player.Page().Eval(script)
		if err != nil {
			a.win.Update(func() { a.errorText = locales.ErrPlayer(err.Error()) })
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
	a.evalPlayer("video.pause();setSubtitles(null);window.previewError='';video.src=" + string(value) + ";video.load();" + a.volumeScript() + a.rateScript())
}
func (a *NativeApp) requestExit() {
	if a.busy {
		go func() {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Title: locales.ExitTitle(), Message: locales.ExitTaskRunning(), Buttons: []string{locales.ExitWait(), locales.ExitCancelTask(), locales.ExitBack()}, CancelButton: 2})
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
	if a.guardUnsaved(a.requestExit) {
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
func (a *NativeApp) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { a.activate() }},
		mygo.Separator(),
		{Label: locales.TrayExit(), Click: func(*mygo.MenuItem, *mygo.Window) { a.requestExit() }},
	})
}
func (a *NativeApp) makeTray() error {
	image, err := trayLogo(mygo.Screen.PrimaryDisplay().ScaleFactor)
	if err != nil {
		return err
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "onesub-main", Icon: image, ToolTip: "OneSub", Menu: a.trayMenu()})
	if err != nil {
		return err
	}
	a.tray = tray
	// Windows reports every left-button release, so a double click opens the
	// window as a single click does; the right button keeps the menu.
	tray.OnClick(a.activate)
	return nil
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
	// "OneSub.exe mcp" is the MCP server a client starts, beside a window
	// that may be open: it takes no instance lock and shows nothing.
	if len(os.Args) == 2 && os.Args[1] == mcpArgument {
		if err := serveMCP(); err != nil {
			log.Fatal(err)
		}
		return
	}
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
	// The interface language is chosen before any web view exists: WebView2
	// keeps the language of its environment, which the first web view makes.
	mygo.App.SetLocale(prefs.UILanguage)
	// The title bar, the menus and the native view follow the saved appearance.
	mygo.Theme.SetSource(mygo.ThemeSource(prefs.Appearance))
	preview, err := media.NewServer()
	if err != nil {
		log.Fatal(err)
	}
	defer preview.Close()
	a := &NativeApp{resources: resources, dataRoot: data, settings: prefs, settingsError: prefsErr != nil, preview: preview, worker: subtitle.Client{Executable: filepath.Join(resources, "subtitle-core", "OneSub.SubtitleCore.exe"), Scratch: scratch}, probe: filepath.Join(resources, "media-tools", "ffprobe.exe"), ffmpeg: filepath.Join(resources, "media-tools", "ffmpeg.exe"), splitX: float32(prefs.PlayerWidth), splitY: float32(prefs.TopHeight), bottomSplit: float32(prefs.EditorWidth), styleSplit: float32(prefs.StyleWidth), selected: -1, paused: true, activeCue: -1, followRow: -1, locateCue: -1, modelStates: map[string]string{}, exportFormat: "srt"}
	if prefsErr != nil {
		a.errorText = locales.ErrSettingsUnreadable()
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
		a.importPaths([]string{path})
	})
	mygo.App.OnLocaleChanged(func(string) {
		if a.tray != nil {
			a.tray.SetMenu(a.trayMenu())
		}
	})
	mygo.App.WhenReady(func() {
		display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
		bounds := initialWindowBounds(display.WorkArea)
		// Under the native view and the player's page is black: where the page
		// lags behind a fast resize of its pane, black shows, as the page is.
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "OneSub", X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height, MinWidth: min(900, bounds.Width), MinHeight: min(640, bounds.Height), StateKey: "main", BackgroundColor: "#000000", Content: ui.View(a.view)})
		// Black fills the player until its page paints, as the page itself is.
		a.player, err = a.win.NewWebView(mygo.WebViewOptions{URL: "/", BackgroundColor: "#000000"})
		if err != nil {
			a.errorText = err.Error()
		} else {
			a.player.Page().OnDOMReady(func() {
				a.win.Update(func() {
					a.playerReady = true
					a.evalPlayer(a.volumeScript() + a.rateScript())
					a.loadVideo()
				})
			})
		}
		if err = a.makeTray(); err != nil {
			a.errorText = locales.ErrTrayUnavailable()
		}
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.importPaths(append([]string{}, e.Paths...))
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
		a.reloadTranslationProfiles()
		a.reloadFonts()
		fontContext, stopFonts := context.WithCancel(context.Background())
		mygo.App.OnQuit(stopFonts)
		go styles.WatchFonts(fontContext, func() { a.win.Update(a.reloadFonts) })
		if len(a.openOnReady) > 0 {
			paths := a.openOnReady
			a.openOnReady = nil
			a.importPaths(paths)
		} else if a.settings.TranslationRecovery != "" {
			a.restoreTranslationProgress()
		}
		go a.syncPlayer()
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// syncPlayer mirrors the page's playback state into the native controls.
func (a *NativeApp) syncPlayer() {
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
			// Playing again follows the playhead again, from its cue.
			if a.paused && !result.Paused {
				a.followPaused, a.followRow, a.activeCue = false, -1, -1
			}
			a.position = result.Position
			a.duration = result.Duration
			a.paused = result.Paused
			// A page error shows once, as it happens, not again every poll.
			if result.Error != a.playerError {
				a.playerError = result.Error
				if result.Error != "" {
					a.errorText = playerErrorText(result.Error)
				}
			}
		})
	}
}
