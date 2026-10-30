package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OneSub is also a Model Context Protocol server, so that any MCP client
// (an AI agent) can transcribe and translate subtitles with it. It is the
// official Go SDK's server over the stdio transport, which MCP defines for
// local servers a client starts: "OneSub.exe mcp". Messages are on stdin
// and stdout only; logs go to stderr. The tools run what the window runs,
// with the user's settings, models and translation key, and never
// overwrite a file.

// mcpArgument is the command-line argument that starts the server.
const mcpArgument = "mcp"

// mcpConfig is the client configuration that starts this executable's
// server, in the "mcpServers" form most MCP clients read.
func mcpConfig() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "OneSub.exe"
	}
	data, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"onesub": map[string]any{"command": exe, "args": []string{mcpArgument}}}}, "", "  ")
	return string(data)
}

type mcpServer struct {
	tools    toolset
	settings string // settings.json, read at every call so the window's choices apply
}

// runMCP serves until the client closes stdin.
func runMCP(resources, dataRoot, ffmpeg string) error {
	log.SetOutput(os.Stderr)
	server, err := newMCPServer(resources, dataRoot, ffmpeg)
	if err != nil {
		return err
	}
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// newMCPServer is the server with OneSub's tools, for any transport.
func newMCPServer(resources, dataRoot, ffmpeg string) (*mcp.Server, error) {
	scratch := filepath.Join(dataRoot, "cache", "worker")
	if err := os.MkdirAll(scratch, 0700); err != nil {
		return nil, err
	}
	s := mcpServer{
		tools: toolset{
			resources: resources, dataRoot: dataRoot, ffmpeg: ffmpeg,
			worker: subtitle.Client{Executable: filepath.Join(resources, "subtitle-core", "OneSub.SubtitleCore.exe"), Scratch: scratch},
			styles: styles.Client{Executable: filepath.Join(resources, "style-core", "OneSub.StyleCore.exe"), Scratch: scratch},
		},
		settings: filepath.Join(dataRoot, "settings.json"),
	}
	if v := os.Getenv("ONESUB_SUBTITLE_WORKER"); v != "" {
		s.tools.worker.Executable = v
	}
	if v := os.Getenv("ONESUB_STYLE_WORKER"); v != "" {
		s.tools.styles.Executable = v
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "onesub", Title: "OneSub", Version: "0.0.1+" + BuildNumber}, &mcp.ServerOptions{
		Instructions: "OneSub makes subtitles on this computer. transcribe_media turns speech in a video or audio file into a subtitle file with the local OneAsr models; translate_subtitles translates a subtitle file with the AI service set in OneSub (it may cost money). Paths are absolute local paths. Results are new files beside the input; nothing is overwritten.",
	})
	no, yes := false, true
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_languages",
		Description: "List the source languages transcribe_media accepts and the target languages translate_subtitles accepts, as IDs with their names.",
		Annotations: &mcp.ToolAnnotations{Title: "List languages", ReadOnlyHint: true, OpenWorldHint: &no},
	}, s.listLanguages)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "transcribe_media",
		Description: "Recognize the speech of a local video or audio file and save it as a subtitle file (SRT, VTT or ASS), with word-aligned timings. Runs locally with the models chosen in OneSub; long files take minutes. Returns the new file's path.",
		Annotations: &mcp.ToolAnnotations{Title: "Transcribe media", DestructiveHint: &no, OpenWorldHint: &no},
	}, s.transcribe)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "translate_subtitles",
		Description: "Translate a local subtitle file (SRT, VTT, ASS or SSA) with the AI translation service set in OneSub, check the result, and save it as a new subtitle file. Sends the subtitle text to that service, which may charge for it. Returns the new file's path.",
		Annotations: &mcp.ToolAnnotations{Title: "Translate subtitles", DestructiveHint: &no, OpenWorldHint: &yes},
	}, s.translate)
	return server, nil
}

func (s mcpServer) prefs() (settings.Settings, error) {
	return settings.Load(s.settings)
}

// progress tells the client how far a call is, when it asked to hear.
func progress(ctx context.Context, req *mcp.CallToolRequest, done, total float64, message string) {
	token := req.Params.GetProgressToken()
	if token == nil {
		return
	}
	req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: token, Progress: done, Total: total, Message: message})
}

type languageItem struct {
	ID   string `json:"id" jsonschema:"the value to pass"`
	Name string `json:"name" jsonschema:"the language and its country or region"`
}
type languagesOut struct {
	Source []languageItem `json:"source" jsonschema:"languages transcribe_media recognizes"`
	Target []languageItem `json:"target" jsonschema:"languages translate_subtitles translates into"`
}

func (s mcpServer) listLanguages(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, languagesOut, error) {
	out := languagesOut{}
	for _, o := range sourceLanguages() {
		out.Source = append(out.Source, languageItem{o.ID, o.Label})
	}
	for _, o := range targetLanguages() {
		out.Target = append(out.Target, languageItem{o.ID, o.Label})
	}
	return nil, out, nil
}

type transcribeIn struct {
	MediaPath string `json:"media_path" jsonschema:"absolute path of the video or audio file"`
	Language  string `json:"language,omitempty" jsonschema:"spoken language ID from list_languages; default: the one set in OneSub"`
	Format    string `json:"format,omitempty" jsonschema:"srt, vtt or ass; default: the one set in OneSub"`
	OutputDir string `json:"output_dir,omitempty" jsonschema:"absolute folder for the subtitle file; default: the one set in OneSub, else the media's folder"`
}
type subtitleOut struct {
	SubtitlePath string   `json:"subtitle_path" jsonschema:"the new subtitle file"`
	Cues         int      `json:"cues" jsonschema:"the number of subtitles in it"`
	Warnings     []string `json:"warnings,omitempty" jsonschema:"what the user may want to check"`
}

func (s mcpServer) transcribe(ctx context.Context, req *mcp.CallToolRequest, in transcribeIn) (*mcp.CallToolResult, subtitleOut, error) {
	prefs, err := s.prefs()
	if err != nil {
		return nil, subtitleOut{}, err
	}
	if !filepath.IsAbs(in.MediaPath) {
		return nil, subtitleOut{}, errors.New("media_path must be an absolute path")
	}
	if _, err = os.Stat(in.MediaPath); err != nil {
		return nil, subtitleOut{}, err
	}
	if in.Language != "" {
		known := false
		for _, o := range sourceLanguages() {
			known = known || o.ID == in.Language
		}
		if !known {
			return nil, subtitleOut{}, fmt.Errorf("unknown language %q; see list_languages", in.Language)
		}
		prefs.Language = in.Language
	}
	if in.Format != "" {
		prefs.RecognitionFormat = strings.ToLower(in.Format)
	}
	if in.OutputDir != "" {
		if !filepath.IsAbs(in.OutputDir) {
			return nil, subtitleOut{}, errors.New("output_dir must be an absolute path")
		}
		prefs.RecognitionDir = in.OutputDir
	}
	srt, _, cleanup, err := s.tools.recognize(ctx, prefs, in.MediaPath, func(step recognitionStep) {
		done, total := float64(step.Done), float64(step.Total)
		if step.Total < 2 {
			done, total = 0, 0
		}
		progress(ctx, req, done, total, recognitionStepLabel(step))
	})
	if err != nil {
		return nil, subtitleOut{}, err
	}
	defer cleanup()
	style := styles.Style{}
	if prefs.RecognitionFormat == "ass" {
		if prefs.LastStyle != nil {
			style = *prefs.LastStyle
		} else if catalog, err := s.tools.styles.Catalog(ctx); err == nil {
			style = catalog.DefaultStyle
		} else {
			return nil, subtitleOut{}, err
		}
	}
	data, repaired, err := s.tools.recognitionFile(ctx, srt, prefs, style, 0)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	path, err := publishRecognition(in.MediaPath, prefs.RecognitionDir, prefs.RecognitionFormat, data)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	doc, err := s.tools.worker.Read(ctx, path)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	out := subtitleOut{SubtitlePath: path, Cues: len(doc.Cues)}
	if repaired > 0 {
		out.Warnings = []string{locales.RecognitionRepaired(repaired)}
	}
	return nil, out, nil
}

type translateIn struct {
	SubtitlePath   string `json:"subtitle_path" jsonschema:"absolute path of the subtitle file"`
	TargetLanguage string `json:"target_language,omitempty" jsonschema:"target language ID from list_languages; default: the one set in OneSub"`
	OutputPath     string `json:"output_path,omitempty" jsonschema:"absolute path of the new file, which must not exist; default: beside the input, named after the target language"`
}

func (s mcpServer) translate(ctx context.Context, req *mcp.CallToolRequest, in translateIn) (*mcp.CallToolResult, subtitleOut, error) {
	prefs, err := s.prefs()
	if err != nil {
		return nil, subtitleOut{}, err
	}
	if !filepath.IsAbs(in.SubtitlePath) || (in.OutputPath != "" && !filepath.IsAbs(in.OutputPath)) {
		return nil, subtitleOut{}, errors.New("paths must be absolute")
	}
	if prefs.ProviderURL == "" || prefs.ProviderModel == "" {
		return nil, subtitleOut{}, errors.New("no translation service is set in OneSub: open OneSub, Settings, Translation service")
	}
	if in.TargetLanguage != "" {
		known := false
		for _, o := range targetLanguages() {
			known = known || o.ID == in.TargetLanguage
		}
		if !known {
			return nil, subtitleOut{}, fmt.Errorf("unknown target language %q; see list_languages", in.TargetLanguage)
		}
		prefs.TargetLanguage = in.TargetLanguage
	}
	key, err := security.ReadKey(prefs.CredentialScope())
	if err != nil {
		return nil, subtitleOut{}, err
	}
	doc, err := s.tools.worker.Read(ctx, in.SubtitlePath)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	root, checker := s.translationTools()
	config := translator.Config{Endpoint: prefs.ProviderURL, API: prefs.ProviderAPI, Model: prefs.ProviderModel, Key: key, TargetLanguage: prefs.TargetLanguage, SkillRoot: root, Glossary: prefs.Glossary, Domain3D: prefs.Domain3D, ProfileID: prefs.TranslationProfile, ProfilesDir: filepath.Join(s.tools.dataRoot, "translation-styles"), OptimizeTimeline: prefs.OptimizeTimeline, MaxRequests: int(prefs.MaxRequests), CacheDir: filepath.Join(s.tools.dataRoot, "cache", "translation")}
	config.Concurrency, config.BatchSize = prefs.TranslationConcurrency, prefs.TranslationBatchSize
	if info := prefs.ModelInfo(); info != nil {
		config.MaxOutputTokens = info.MaxOutputTokens
	}
	// Progress names the stage and the subtitles translated, as the window
	// shows them. MCP progress must only increase, and the stages count
	// different things, so the value counts notifications and the message
	// carries the counts.
	reported, sent := "", 0
	config.Observe = func(ev translator.Event) {
		message := locales.TranslationAnalyzing()
		switch ev.Stage {
		case "segment":
			message = locales.TranslationSegmenting(ev.Done, ev.Total)
		case "translate":
			message = locales.TranslationProgress(ev.Done, ev.Total)
		}
		if message != reported {
			reported = message
			sent++
			progress(ctx, req, float64(sent), 0, message)
		}
	}
	result, err := translator.Translate(ctx, config, doc)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	checks, err := checker.Check(ctx, doc, result.Document, prefs.TargetLanguage, !result.Report.Rebuilt)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	// A translation that fails the full-file check is not saved, as the
	// window does not accept it.
	if len(checks.Errors) > 0 {
		return nil, subtitleOut{}, errors.New("the translation failed its checks: " + strings.Join(checks.Errors, "; "))
	}
	out, err := subtitle.SelectOutput(result.Document, subtitle.OutputOptions{Mode: "translation"})
	if err != nil {
		return nil, subtitleOut{}, err
	}
	data, err := s.tools.worker.Write(ctx, out)
	if err != nil {
		return nil, subtitleOut{}, err
	}
	path := in.OutputPath
	if path == "" {
		ext := strings.TrimPrefix(filepath.Ext(in.SubtitlePath), ".")
		stem := strings.TrimSuffix(filepath.Base(in.SubtitlePath), filepath.Ext(in.SubtitlePath)) + "-" + translationSuffix(prefs.TargetLanguage)
		dir := prefs.TranslationDir
		if dir == "" {
			dir = filepath.Dir(in.SubtitlePath)
		}
		path, err = publishBeside(dir, stem, ext, data)
	} else {
		err = subtitle.Publish(path, data)
	}
	if err != nil {
		return nil, subtitleOut{}, err
	}
	warnings := append(append([]string{}, checks.Warnings...), result.Report.Warnings...)
	return nil, subtitleOut{SubtitlePath: path, Cues: len(out.Cues), Warnings: warnings}, nil
}

// translationTools are the subtitle skill's rules and its checker, as the
// window finds them.
func (s mcpServer) translationTools() (string, translator.Checker) {
	root := filepath.Join(s.tools.resources, "srt-subtitle-translator")
	if v := os.Getenv("ONESUB_SKILL_ROOT"); v != "" {
		root = v
	}
	executable := filepath.Join(s.tools.resources, "skill-tools", "OneSub.SkillTools.exe")
	if v := os.Getenv("ONESUB_SKILL_WORKER"); v != "" {
		executable = v
	}
	return root, translator.Checker{Executable: executable, SkillRoot: root, Scratch: s.tools.worker.Scratch, Subtitle: s.tools.worker}
}

// serveMCP finds the app's folders as the window does, then serves.
func serveMCP() error {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return err
	}
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	if v := os.Getenv("ONESUB_DATA_ROOT"); v != "" {
		data = v
	}
	// Messages, progress and errors in the language the window shows.
	if prefs, err := settings.Load(filepath.Join(data, "settings.json")); err == nil {
		mygo.App.SetLocale(prefs.UILanguage)
	}
	ffmpeg := filepath.Join(resources, "media-tools", "ffmpeg.exe")
	if v := os.Getenv("ONESUB_FFMPEG"); v != "" {
		ffmpeg = v
	}
	return runMCP(resources, data, ffmpeg)
}
