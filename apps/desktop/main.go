package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed icons/*.svg
var iconFiles embed.FS

//go:embed branding/logo.png
var appLogo []byte

//go:embed player/index.html player/generated
var playerFiles embed.FS
var icons = map[string]*ui.SVG{}
var BuildNumber = "native-development"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "diarize" {
		if err := runDiarize(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// "OneSub.exe mcp" is the MCP server a client starts, beside a window
	// that may be open: it takes no instance lock and shows nothing.
	// "OneSub.exe punctuate" is the child that restores Paraformer's
	// punctuation, also without a window or lock.
	if len(os.Args) >= 2 && os.Args[1] == punctuateArgument {
		if err := runPunctuate(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == mcpArgument {
		if err := serveMCP(); err != nil {
			log.Fatal(err)
		}
		return
	}
	// Keep WebView2's supported composition path. Disabling DirectComposition
	// leaves composition-hosted media invisible on newer WebView2 runtimes,
	// even while the page decodes video and reports no playback error.
	// Configure the video presentation path before MyGo initializes WebView2.
	if err := configurePlayerRuntime(); err != nil {
		log.Fatal(err)
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
	a := &NativeApp{resources: resources, dataRoot: data, settings: prefs, settingsError: prefsErr != nil, preview: preview, worker: subtitle.Client{Executable: filepath.Join(resources, "subtitle-core", "OneSub.SubtitleCore.exe"), Scratch: scratch}, probe: filepath.Join(resources, "media-tools", "ffprobe.exe"), ffmpeg: filepath.Join(resources, "media-tools", "ffmpeg.exe"), splitX: float32(prefs.PlayerWidth), splitY: float32(prefs.TopHeight), editorSplit: float32(prefs.EditorWidth), subtitleSplit: float32(prefs.SubtitleHeight), selected: -1, paused: true, activeCue: -1, locateCue: -1, modelStates: map[string]string{}, exportFormat: "srt"}
	a.refreshDownloadedAsrModels()
	if prefsErr != nil {
		a.showError("settings", locales.ErrSettingsUnreadable())
	}
	a.restoreBatchSources()
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
		a.acceptFiles([]string{path})
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
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "OneSub", X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height, MinWidth: min(minimumWindowWidth, bounds.Width), MinHeight: min(640, bounds.Height), StateKey: "main", BackgroundColor: "#000000", Content: ui.View(a.view)})
		a.win.OnLeaveFullScreen(func() {
			a.win.Update(func() { a.playerFullscreen = false })
		})
		// Black fills the player until its page paints, as the page itself is.
		a.player, err = a.win.NewWebView(mygo.WebViewOptions{URL: "/", BackgroundColor: "#000000"})
		if err != nil {
			a.showError("player", err.Error())
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
			a.showError("tray", locales.ErrTrayUnavailable())
		}
		a.win.OnFocus(a.refreshDownloadedAsrModels)
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.acceptFiles(append([]string{}, e.Paths...))
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
			a.acceptFiles(paths)
		} else if a.settings.TranslationRecovery != "" {
			a.restoreTranslationProgress()
		} else if a.settings.Processing != nil {
			a.restoreProcessing()
		}
		a.startUpdates()
		go a.syncPlayer()
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
