package main

import (
	"context"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// NativeApp keeps model data, not ui.Element values. The view and callbacks run
// on MyGo's main thread; background results enter through Window.Update.
type NativeApp struct {
	speakers                             speakerUI
	playerFullscreen                     bool
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
	storage                              storageState
	updates                              appUpdates
	errorText, status                    string
	errorScope                           string    // the operation whose error is currently displayed
	mediaGeneration                      uint64    // rejects out-of-order media load commands
	errorShown                           string    // the error the status bar shows, since errorSince
	errorSince                           time.Time // when it began to show
	playerError                          string    // the page's last error code
	rate                                 float64   // the playback speed; 0 is normal
	loop                                 bool      // the video starts again at its end
	busy                                 bool
	taskTimes                            [timingCount]*taskTiming
	activeTaskTiming                     *taskTiming
	choosingFile                         bool
	cancel                               context.CancelFunc
	pending                              *importing.Loaded
	candidatesOpen                       bool
	splitX, splitY, subtitleSplit        float32
	editorSplit                          float32 // middle column's share of the room right of the player
	styleTab, providerSettingsTab        int
	workflowTab                          int // 0 recognition, 1 translation
	recognitionMode                      int // 0 current media, 1 file queue; both use settings
	workflowScroll, styleScroll          ui.ScrollState
	list                                 ui.ListState
	subtitleGuide                        subtitleLengthGuide
	selected                             int
	query                                string
	subtitleChecked                      [2]map[string]bool // batch checks are independent of the editing row
	retranslation, retranslationNext     *retranslationState
	readingSnapshot                      *project.Snapshot
	countSnapshot                        *project.Snapshot
	originalCount, optimizedCount        int
	readingTab, readingIssue             int  // issue defaults to long; independent of the active track
	readingExpanded                      bool // opt in per session; all reading controls start folded
	reading                              []subtitle.CueReadingLoad
	problems                             subtitleProblemState
	splitSkipped                         map[string]string
	findReplace                          findReplaceState
	draft                                string
	start, end                           float64
	editPending, composing, editing      bool
	editDue                              time.Time
	savedHash, savedPath                 [3]string // original, translation, proofreading source
	unsaved                              [3]bool
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
	locateCue                            int       // a cue to center, or -1
	browseUntil                          time.Time // until when the user's browsing keeps the viewport
	browseHeld                           bool      // manual scrolling persists until the locate button
	followRow                            int       // the row the playback follow centered, or -1
	followPending                        bool      // the follow owes the viewport a move back
	followHeld                           bool      // explicit select-only holds selection until locate/seek
	rowClick                             *subtitleRowClick
	pendingClick                         *subtitleClick
	frameAspect                          float64 // the video's display width / height
	modelStates                          map[string]string
	downloadedAsr                        []option // complete local ASR files; nil refreshes once
	modelSelection                       string
	modelOpen                            bool
	modelRecycle                         string // model ID awaiting a native recycle confirmation
	exportOpen                           bool
	exportFormat                         string
	exportLines                          string // all, upper or lower; export-only projection
	quitAfterTask                        bool
	composeOpen                          bool
	exportSkipMissing                    bool
	missingSaveOpen                      bool
	missingSaveNext                      func()
	compressOpen                         bool
	compressRunning                      bool
	audioExtractRunning                  bool
	composeRunning                       bool
	composeInBackground                  bool // video composition keeps running while the player is available
	recognitionRunning                   bool
	recognitionStarted                   time.Time
	recognitionStep                      recognitionStep
	recognitionRoute                     recognitionRoute
	batchItems                           []batchItem
	batchList                            ui.ListState
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
	subtitleTab                          int
	proofreading                         bool
	proofreadingSourceTracked            bool
	proofreadingPreviousTab              int
	proofreadingImport                   *proofreadingImport
	proofreadingFiles                    [3]*proofreadingFile
	editRebuiltSource                    bool
	insertOpen                           bool
	insertPosition                       int
	insertStart, insertEnd               float64
	insertSource, insertTranslation      string
	openOnReady                          []string
	providerKey, keyStatus               string
	providerKeyVisible                   bool
	modelQuery                           string
	modelEdit                            providerModelDraft
	modelList                            ui.ListState
	modelsError                          string
	modelsLoading                        bool
	modelsGeneration                     uint64
	modelChecks                          map[string]*providerModelCheck
	providerDelete                       bool
	fontCatalogLoading                   bool
	translationReview                    bool // the check results of the last translation are shown
	translationCandidate                 *translationCandidate
	translationProfiles                  []translator.Profile
	profilesError                        string
	translationCheckTab                  int
	translationIssueList                 ui.ListState
	translationIssueSelected             int
	translationIssueQuery                string
}
