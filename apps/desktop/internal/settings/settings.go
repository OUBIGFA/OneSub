// Package settings persists user choices separately from projects and secrets.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
)

type Settings struct {
	Version           int               `json:"version"`
	CloseToTray       bool              `json:"closeToTray"`
	ModelPaths        map[string]string `json:"modelPaths"`
	AsrModel          string            `json:"asrModel"`
	Aligner           string            `json:"aligner"`
	Backend           string            `json:"backend"`
	Language          string            `json:"language"`
	ChunkSeconds      float64           `json:"chunkSeconds"`
	MaxTokens         float64           `json:"maxTokens"`
	VocalSeparation   bool              `json:"vocalSeparation"`
	Hotwords          string            `json:"hotwords"`
	ProviderURL       string            `json:"providerUrl"`
	ProviderModel     string            `json:"providerModel"`
	TargetLanguage    string            `json:"targetLanguage"`
	Glossary          string            `json:"glossary"`
	Domain3D          bool              `json:"domain3D"`
	MaxRequests       float64           `json:"maxRequests"`
	OutputDir         string            `json:"outputDir"`
	LastStyle         *styles.Style     `json:"lastStyle,omitempty"`
	StylePreset       string            `json:"stylePreset,omitempty"`
	SingleClickAction string            `json:"singleClickAction"`
	DoubleClickAction string            `json:"doubleClickAction"`
	FollowPlayback    bool              `json:"followPlayback"`
	RecentProjects    []string          `json:"recentProjects"`
}

func Default() Settings {
	return Settings{Version: 1, ModelPaths: map[string]string{}, AsrModel: "Qwen3-ASR-0.6B-hf", Aligner: "omniASR-CTC-300M-v2-hf", Backend: "auto", Language: "zh", ChunkSeconds: 60, MaxTokens: 2048, TargetLanguage: "zh-CN", MaxRequests: 100, SingleClickAction: "seek-pause", DoubleClickAction: "seek-play", FollowPlayback: true}
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
	if s.Version != 1 {
		return Default(), errors.New("不支持此设置版本；原设置未修改")
	}
	if s.ModelPaths == nil {
		s.ModelPaths = map[string]string{}
	}
	return s, nil
}
func Save(path string, s Settings) error {
	if s.ChunkSeconds < 30 || s.ChunkSeconds > 180 {
		return errors.New("音频分块时长须为 30–180 秒")
	}
	if s.MaxTokens < 1 || s.MaxTokens > 32768 {
		return errors.New("最大生成 Token 数须为 1–32768")
	}
	if s.MaxRequests < 1 || s.MaxRequests > 10000 {
		return errors.New("本次翻译请求上限须为 1–10000")
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
