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
	Version                int                    `json:"version"`
	UILanguage             string                 `json:"uiLanguage"` // "" follows the system
	Appearance             string                 `json:"appearance"` // "dark", "light" or "system"
	CloseToTray            bool                   `json:"closeToTray"`
	ModelPaths             map[string]string      `json:"modelPaths"`
	AsrModel               string                 `json:"asrModel"`
	Aligner                string                 `json:"aligner"`
	Backend                string                 `json:"backend"`
	Language               string                 `json:"language"`
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
	Volume                 float64                `json:"volume"` // percent
	Muted                  bool                   `json:"muted"`
	SingleClickAction      string                 `json:"singleClickAction"`
	DoubleClickAction      string                 `json:"doubleClickAction"`
	FollowPlayback         bool                   `json:"followPlayback"`
	// The workbench panes, in percent: the player's width beside the list,
	// the upper area's height, and the style's width in the lower area.
	// EditorWidth is the editor's share of the room right of the style,
	// beside recognition and translation.
	PlayerWidth float64 `json:"playerWidth"`
	TopHeight   float64 `json:"topHeight"`
	EditorWidth float64 `json:"editorWidth"`
	StyleWidth  float64 `json:"styleWidth"`
	// Recognized subtitles lose the punctuation that ends their lines,
	// except the marks in KeepPunctuation, in ASCII or full width.
	StripPunctuation bool   `json:"stripPunctuation"`
	KeepPunctuation  string `json:"keepPunctuation"`
}

// version is that of the settings written now. Version 1 had full volume
// and SmartSub's shadow as defaults; Load moves them to version 2's.
const version = 5

func Default() Settings {
	return Settings{TranslationConcurrency: translator.DefaultConcurrency, TranslationBatchSize: translator.DefaultBatchSize, SubtitleOutput: subtitle.OutputOptions{SourceTrack: "original"}, Version: version, CloseToTray: true, Appearance: "dark", ModelPaths: map[string]string{}, AsrModel: "Qwen3-ASR-0.6B-hf", Aligner: "omniASR-CTC-300M-v2-hf", Backend: "auto", Language: "zh", RecognitionFormat: "srt", ChunkSeconds: 60, MaxTokens: 2048, ProviderAPI: "chat-completions", TargetLanguage: "zh-CN", MaxRequests: 100, OptimizeTimeline: true, ComposeMode: "hard-mp4", ComposeQuality: "crf20", Volume: 50, SingleClickAction: "seek-pause", DoubleClickAction: "seek-play", FollowPlayback: true, PlayerWidth: 60, TopHeight: 64, EditorWidth: 400.0 / 7, StyleWidth: 30, StripPunctuation: true, KeepPunctuation: "?"}
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
	s.restoreProviders()
	s.Volume = max(0, min(100, s.Volume))
	// A pane is never made too small to show or to drag back.
	d := Default()
	for _, pane := range []struct{ value, fallback *float64 }{{&s.PlayerWidth, &d.PlayerWidth}, {&s.TopHeight, &d.TopHeight}, {&s.EditorWidth, &d.EditorWidth}, {&s.StyleWidth, &d.StyleWidth}} {
		if !(*pane.value >= 10 && *pane.value <= 90) {
			*pane.value = *pane.fallback
		}
	}
	return s, nil
}

// Version 5 swaps style and editor and applies the new 1.5:2:1.5 lower layout
// once on upgrade. The old percentages belonged to different split positions;
// upper panes and all unrelated preferences remain intact. Later drags persist.
func upgrade(s *Settings) {
	d := Default()
	s.EditorWidth, s.StyleWidth = d.EditorWidth, d.StyleWidth
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
