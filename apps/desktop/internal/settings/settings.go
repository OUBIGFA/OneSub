// Package settings persists user choices separately from projects and secrets.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// UserPreset is a named copy of a style the user saved. Built-in SmartSub
// presets are never changed; editing starts from a copy.
type UserPreset struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Style styles.Style `json:"style"`
}

type Settings struct {
	Version            int                    `json:"version"`
	UILanguage         string                 `json:"uiLanguage"` // "" follows the system
	Appearance         string                 `json:"appearance"` // "dark", "light" or "system"
	CloseToTray        bool                   `json:"closeToTray"`
	ModelPaths         map[string]string      `json:"modelPaths"`
	AsrModel           string                 `json:"asrModel"`
	Aligner            string                 `json:"aligner"`
	Backend            string                 `json:"backend"`
	Language           string                 `json:"language"`
	ChunkSeconds       float64                `json:"chunkSeconds"`
	MaxTokens          float64                `json:"maxTokens"`
	VocalSeparation    bool                   `json:"vocalSeparation"`
	Hotwords           string                 `json:"hotwords"`
	ProviderAPI        string                 `json:"providerApi"` // translator.APIChatCompletions or APIResponses
	ProviderURL        string                 `json:"providerUrl"`
	ProviderModel      string                 `json:"providerModel"`
	TargetLanguage     string                 `json:"targetLanguage"`
	Glossary           string                 `json:"glossary"`
	Domain3D           bool                   `json:"domain3D"`
	TranslationProfile string                 `json:"translationProfile"`
	OptimizeTimeline   bool                   `json:"optimizeTimeline"`
	MaxRequests        float64                `json:"maxRequests"`
	RecognitionFormat  string                 `json:"recognitionFormat"`
	RecognitionDir     string                 `json:"recognitionDir"` // empty: beside the media
	TranslationDir     string                 `json:"translationDir"` // empty: beside the subtitle or the media
	OutputDir          string                 `json:"outputDir"`
	LastStyle          *styles.Style          `json:"lastStyle,omitempty"`
	StylePreset        string                 `json:"stylePreset,omitempty"` // preset ID
	UserPresets        []UserPreset           `json:"userPresets,omitempty"`
	SubtitleOutput     subtitle.OutputOptions `json:"subtitleOutput"`
	SecondaryStyle     *styles.Style          `json:"secondaryStyle,omitempty"`
	SecondaryPreset    string                 `json:"secondaryPreset,omitempty"`
	ComposeMode        string                 `json:"composeMode"`
	ComposeQuality     string                 `json:"composeQuality"`
	Volume             float64                `json:"volume"` // percent
	Muted              bool                   `json:"muted"`
	SingleClickAction  string                 `json:"singleClickAction"`
	DoubleClickAction  string                 `json:"doubleClickAction"`
	FollowPlayback     bool                   `json:"followPlayback"`
	// The workbench panes, in percent: the player's width beside the list,
	// the upper area's height, and the editor's width beside the style.
	// StyleWidth is the style's share of the room right of the editor,
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
const version = 3

func Default() Settings {
	return Settings{Version: version, Appearance: "dark", ModelPaths: map[string]string{}, AsrModel: "Qwen3-ASR-0.6B-hf", Aligner: "omniASR-CTC-300M-v2-hf", Backend: "auto", Language: "zh", RecognitionFormat: "srt", ChunkSeconds: 60, MaxTokens: 2048, ProviderAPI: "chat-completions", TargetLanguage: "zh-CN", MaxRequests: 100, OptimizeTimeline: true, ComposeMode: "hard-mp4", ComposeQuality: "crf20", Volume: 50, SingleClickAction: "seek-pause", DoubleClickAction: "seek-play", FollowPlayback: true, PlayerWidth: 60, TopHeight: 64, EditorWidth: 20, StyleWidth: 50, StripPunctuation: true, KeepPunctuation: "?"}
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
	if s.Version < 1 || s.Version > version {
		return Default(), errors.New(locales.ErrSettingsVersion())
	}
	if s.Version < version {
		upgrade(&s)
	}
	if s.ModelPaths == nil {
		s.ModelPaths = map[string]string{}
	}
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

// upgrade moves the untouched defaults of earlier versions to the current
// ones. Version 2 has half volume and the default style without a shadow;
// version 3 sizes the editor, the style and recognition with translation
// 1:2:2. Values the user chose stay.
func upgrade(s *Settings) {
	if s.Version >= 2 {
		if s.EditorWidth == 25 && s.StyleWidth > 66.6 && s.StyleWidth < 66.7 {
			s.EditorWidth, s.StyleWidth = 20, 50
		}
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
