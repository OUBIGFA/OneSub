// Package settings persists user choices separately from projects and secrets.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// UserPreset is a named copy of a style the user saved. Built-in SmartSub
// presets are never changed; editing starts from a copy.
type UserPreset struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Style styles.Style `json:"style"`
}

// PendingProcessing records completed pipeline steps, not model secrets or
// generated media. It lets an interrupted recognition/translation reopen safely.
type PendingProcessing struct {
	ProjectPath string    `json:"projectPath"`
	Stage       string    `json:"stage"` // recognize, save-recognition, translate
	Translate   bool      `json:"translate"`
	SavedHash   [2]string `json:"savedHash"`
	SavedPath   [2]string `json:"savedPath"`
}

type Settings struct {
	Version     int    `json:"version"`
	UILanguage  string `json:"uiLanguage"` // "" follows the system
	Appearance  string `json:"appearance"` // "dark", "light" or "system"
	CloseToTray bool   `json:"closeToTray"`
	// AutoUpdate defaults on for new and older settings. An explicit false
	// survives loading; updates never replace the user's data directory.
	AutoUpdate bool              `json:"autoUpdate"`
	ModelPaths map[string]string `json:"modelPaths"`
	AsrModel   string            `json:"asrModel"`
	Aligner    string            `json:"aligner"`
	Backend    string            `json:"backend"`
	// RecognitionMode is "route": detect the language and use the model
	// LanguageModels names for its group (or the group's default model); or
	// "manual": AsrModel with Language, which is "auto" only for a model
	// that detects the language itself.
	RecognitionMode string            `json:"recognitionMode"`
	Language        string            `json:"language"`
	LanguageModels  map[string]string `json:"languageModels"`
	// ManualLanguages remembers the source-language choice per ASR model.
	// A detecting model defaults to auto once; later explicit choices survive
	// switching models or recognition modes.
	ManualLanguages        map[string]string      `json:"manualLanguages,omitempty"`
	ChunkSeconds           float64                `json:"chunkSeconds"`
	MaxTokens              float64                `json:"maxTokens"`
	VocalSeparation        bool                   `json:"vocalSeparation"`
	Hotwords               string                 `json:"hotwords"`
	ProviderAPI            string                 `json:"providerApi"` // translator.APIChatCompletions or APIResponses
	ProviderURL            string                 `json:"providerUrl"`
	ProviderModel          string                 `json:"providerModel"`
	ActiveProvider         string                 `json:"activeProvider,omitempty"`
	Providers              []Provider             `json:"providers,omitempty"`
	TargetLanguage         string                 `json:"targetLanguage"`
	Glossary               string                 `json:"glossary"`
	Domain3D               bool                   `json:"domain3D"`
	TranslationProfile     string                 `json:"translationProfile"`
	OptimizeTimeline       bool                   `json:"optimizeTimeline"`
	MaxRequests            float64                `json:"maxRequests"`
	TranslationConcurrency int                    `json:"translationConcurrency"`
	TranslationBatchSize   int                    `json:"translationBatchSize"`
	RecognitionFormat      string                 `json:"recognitionFormat"`
	RecognitionDir         string                 `json:"recognitionDir"` // empty: beside the media
	TranslationDir         string                 `json:"translationDir"` // empty: beside the subtitle or the media
	TranslationRecovery    string                 `json:"translationRecovery,omitempty"`
	Processing             *PendingProcessing     `json:"processing,omitempty"`
	ComposeAskPath         bool                   `json:"composeAskPath"` // prompt for each export when true
	OutputDir              string                 `json:"outputDir"`      // empty: source video directory
	LastStyle              *styles.Style          `json:"lastStyle,omitempty"`
	StylePreset            string                 `json:"stylePreset,omitempty"` // preset ID
	UserPresets            []UserPreset           `json:"userPresets,omitempty"`
	SubtitleOutput         subtitle.OutputOptions `json:"subtitleOutput"`
	SecondaryStyle         *styles.Style          `json:"secondaryStyle,omitempty"`
	SecondaryPreset        string                 `json:"secondaryPreset,omitempty"`
	ComposeMode            string                 `json:"composeMode"`
	ComposeQuality         string                 `json:"composeQuality"`
	CompressionFormat      string                 `json:"compressionFormat"`
	CompressionQuality     string                 `json:"compressionQuality"`
	Volume                 float64                `json:"volume"` // percent
	Muted                  bool                   `json:"muted"`
	SingleClickAction      string                 `json:"singleClickAction"`
	DoubleClickAction      string                 `json:"doubleClickAction"`
	FollowPlayback         bool                   `json:"followPlayback"`
	// Percentages: PlayerWidth divides the left column from the other two;
	// EditorWidth divides their remaining room. TopHeight is the player's
	// vertical share; SubtitleHeight independently divides the middle column.
	PlayerWidth    float64 `json:"playerWidth"`
	TopHeight      float64 `json:"topHeight"`
	EditorWidth    float64 `json:"editorWidth"`
	SubtitleHeight float64 `json:"subtitleHeight"`
	// Local translation-output cleanup. Recognition preserves its punctuation;
	// existing saved strip/keep choices move with the controls.
	StripPunctuation bool   `json:"stripPunctuation"`
	KeepPunctuation  string `json:"keepPunctuation"`
	ReplacePeriods   bool   `json:"replacePeriods"`
	// BatchSources are the media files waiting in the recognition queue.
	// The queue survives a restart, so a long list of files is not lost.
	BatchSources []string `json:"batchSources,omitempty"`
}

// version is that of the settings written now. Version 1 had full volume
// and SmartSub's shadow as defaults; Load moves them to version 2's.
const version = 6

func Default() Settings {
	return Settings{TranslationConcurrency: translator.DefaultConcurrency, TranslationBatchSize: translator.DefaultBatchSize, SubtitleOutput: subtitle.OutputOptions{SourceTrack: "original"}, Version: version, CloseToTray: true, AutoUpdate: true, Appearance: "dark", ModelPaths: map[string]string{}, AsrModel: "Qwen3-ASR-0.6B-hf", Aligner: "omniASR-CTC-300M-v2-hf", Backend: "auto", RecognitionMode: "route", Language: "zh", LanguageModels: map[string]string{}, ManualLanguages: map[string]string{}, RecognitionFormat: "srt", ChunkSeconds: 60, MaxTokens: 2048, ProviderAPI: "chat-completions", TargetLanguage: "zh-CN", MaxRequests: 100, OptimizeTimeline: true, ComposeMode: "hard-mp4", ComposeQuality: "crf20", CompressionFormat: "mp4", CompressionQuality: "crf23", Volume: 50, SingleClickAction: "seek-pause", DoubleClickAction: "seek-play", FollowPlayback: true, PlayerWidth: 300.0 / 7, TopHeight: 75, EditorWidth: 75, SubtitleHeight: 75, StripPunctuation: true, ReplacePeriods: true, KeepPunctuation: "?"}
}

// TranslationPunctuation freezes the local output options for a task.
func (s Settings) TranslationPunctuation() *subtitle.PunctuationOptions {
	return &subtitle.PunctuationOptions{StripTrailing: s.StripPunctuation, Keep: s.KeepPunctuation, ReplacePeriods: s.ReplacePeriods}
}

func Load(path string) (Settings, error) {
	s := Default()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	// An absent mode belongs to an older settings file. Do not replace a
	// user's fixed model/language with routing merely by opening it.
	s.RecognitionMode = ""
	if err = json.Unmarshal(raw, &s); err != nil {
		return Default(), err
	}
	if err = s.translationSchedulingError(); err != nil {
		return Default(), err
	}
	if s.Version < 1 || s.Version > version {
		return Default(), errors.New(locales.ErrSettingsVersion())
	}
	if s.Version < version {
		upgrade(&s)
	}
	// The lossless UI choice was retired. Migrate only that choice to the
	// installation default; keep existing CRF levels and pane preferences.
	if s.ComposeQuality == "lossless" {
		s.ComposeQuality = Default().ComposeQuality
	}
	if s.ModelPaths == nil {
		s.ModelPaths = map[string]string{}
	}
	if s.LanguageModels == nil {
		s.LanguageModels = map[string]string{}
	}
	if s.RecognitionMode == "" && s.Language != "auto" {
		s.RecognitionMode = "manual"
	}
	if s.RecognitionMode != "manual" {
		s.RecognitionMode = "route"
	}
	if s.ManualLanguages == nil {
		s.ManualLanguages = map[string]string{}
	}
	if s.RecognitionMode == "manual" {
		// An existing fixed-language workflow is an explicit choice, not
		// a new model selection that should revert to automatic detection.
		if _, remembered := s.ManualLanguages[s.AsrModel]; !remembered {
			s.ManualLanguages[s.AsrModel] = s.Language
		}
	}
	s.restoreProviders()
	// Retire seek-with-current-state without changing pane ratios or other
	// preferences. Both click menus now have the same three explicit actions.
	for _, action := range []*string{&s.SingleClickAction, &s.DoubleClickAction} {
		if *action == "seek" {
			*action = "seek-play"
		}
	}
	s.Volume = max(0, min(100, s.Volume))
	// A pane is never made too small to show or to drag back.
	d := Default()
	for _, pane := range []struct{ value, fallback *float64 }{{&s.PlayerWidth, &d.PlayerWidth}, {&s.TopHeight, &d.TopHeight}, {&s.EditorWidth, &d.EditorWidth}, {&s.SubtitleHeight, &d.SubtitleHeight}} {
		if !(*pane.value >= 10 && *pane.value <= 90) {
			*pane.value = *pane.fallback
		}
	}
	return s, nil
}

// Version 6 changes the split hierarchy to three columns (3:3:1), with two
// independent 3:1 vertical splits. Old ratios describe different dividers;
// reset only that geometry once and retain all other user preferences.
func upgrade(s *Settings) {
	d := Default()
	s.PlayerWidth, s.TopHeight = d.PlayerWidth, d.TopHeight
	s.EditorWidth, s.SubtitleHeight = d.EditorWidth, d.SubtitleHeight
	if s.Version >= 2 {
		s.Version = version
		return
	}
	if s.Volume == 100 {
		s.Volume = 50
	}
	noShadow := func(style *styles.Style, preset string) {
		if style != nil && (preset == "" || preset == "default") && style.BorderStyle == 1 && style.Shadow == 1 && style.BackOpacity == 50 {
			style.Shadow, style.BackOpacity = 0, 0
		}
	}
	noShadow(s.LastStyle, s.StylePreset)
	noShadow(s.SecondaryStyle, s.SecondaryPreset)
	s.Version = version
}
func Save(path string, s Settings) error {
	if err := s.translationSchedulingError(); err != nil {
		return err
	}
	s.SyncProvider()
	if s.RecognitionFormat != "srt" && s.RecognitionFormat != "vtt" && s.RecognitionFormat != "ass" {
		return errors.New(locales.RecognitionFormatInvalid())
	}
	if s.ChunkSeconds < 30 || s.ChunkSeconds > 180 {
		return errors.New(locales.ErrSettingsChunk())
	}
	if s.MaxTokens < 1 || s.MaxTokens > 32768 {
		return errors.New(locales.ErrSettingsMaxTokens())
	}
	if s.MaxRequests < 1 || s.MaxRequests > 10000 {
		return errors.New(locales.ErrSettingsMaxRequests())
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "settings-*.partial")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// Both loading and saving keep fractional, zero and unbounded task settings
// out of the runtime; omitted fields retain Default's values on older installs.
func (s Settings) translationSchedulingError() error {
	if s.TranslationConcurrency < 1 || s.TranslationConcurrency > 10000 {
		return errors.New(locales.ErrTranslationConcurrency())
	}
	if s.TranslationBatchSize < 1 || s.TranslationBatchSize > 10000 {
		return errors.New(locales.ErrTranslationBatchSize())
	}
	return nil
}
