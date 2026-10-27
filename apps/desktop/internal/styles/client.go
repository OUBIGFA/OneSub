// Package styles calls SmartSub's font and ASS modules (with the registered
// font-alias fix) through a one-shot worker. There is deliberately no second ASS builder in Go.
package styles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"os"
	"path/filepath"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

type Style struct {
	FontName        string   `json:"fontName"`
	FontSize        float64  `json:"fontSize"`
	PrimaryColor    string   `json:"primaryColor"`
	OutlineColor    string   `json:"outlineColor"`
	BackColor       string   `json:"backColor"`
	BackOpacity     float64  `json:"backOpacity"`
	Bold            bool     `json:"bold"`
	Italic          bool     `json:"italic"`
	Underline       bool     `json:"underline"`
	BorderStyle     int      `json:"borderStyle"`
	Outline         float64  `json:"outline"`
	Shadow          float64  `json:"shadow"`
	Alignment       int      `json:"alignment"`
	MarginL         float64  `json:"marginL"`
	MarginR         float64  `json:"marginR"`
	MarginV         float64  `json:"marginV"`
	SecondLineColor string   `json:"secondLineColor,omitempty"`
	HighlightColor  string   `json:"highlightColor,omitempty"`
	HighlightTerms  []string `json:"highlightTerms,omitempty"`
	GlowColor       string   `json:"glowColor,omitempty"`
	Glow            float64  `json:"glow,omitempty"`
}
type Preset struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Style Style  `json:"style"`
}
type Font struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Available   bool   `json:"available"`
}
type Catalog struct {
	Presets      []Preset `json:"presets"`
	DefaultStyle Style    `json:"defaultStyle"`
	Fonts        []Font   `json:"fonts"`
}
type Document struct {
	ASS   string `json:"ass"`
	Style Style  `json:"style"`
	Fonts []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"fonts"`
	Report struct {
		MissingGlyphs []string                            `json:"missingGlyphs"`
		Fallbacks     []struct{ From, To, Sample string } `json:"fallbacks"`
	} `json:"report"`
}
type Client struct{ Executable, Scratch string }

func (c Client) Catalog(ctx context.Context) (Catalog, error) {
	var result Catalog
	err := c.call(ctx, "catalog", nil, &result)
	return result, err
}
func (c Client) Build(ctx context.Context, cues []subtitle.Cue, style Style) (Document, error) {
	var result Document
	err := c.call(ctx, "build", map[string]any{"cues": cues, "style": style}, &result)
	return result, err
}
func (c Client) Resolve(ctx context.Context, ass string, style Style) (Document, error) {
	var result Document
	err := c.call(ctx, "resolve", map[string]any{"ass": ass, "style": style}, &result)
	return result, err
}
func (c Client) call(ctx context.Context, operation string, input any, result any) error {
	dir, err := os.MkdirTemp(c.Scratch, "style-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	inputPath, outputPath := filepath.Join(dir, "input.json"), filepath.Join(dir, "output.json")
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if err = os.WriteFile(inputPath, data, 0600); err != nil {
			return err
		}
	}
	request, err := json.Marshal(map[string]any{"protocol": 1, "requestId": filepath.Base(dir), "operation": operation, "inputPath": inputPath, "outputPath": outputPath, "fontsPath": filepath.Join(c.Scratch, "fonts")})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := jobs.CommandContext(ctx, c.Executable)
	cmd.Stdin = bytes.NewReader(request)
	response, runErr := cmd.Output()
	var reply struct {
		Protocol  int
		RequestID string
		OK        bool
		Error     string
	}
	if err = json.Unmarshal(response, &reply); err != nil || reply.Protocol != 1 || reply.RequestID != filepath.Base(dir) {
		return fmt.Errorf("样式工作进程无有效回执：%v", runErr)
	}
	if !reply.OK {
		return fmt.Errorf("样式处理失败：%s", reply.Error)
	}
	if runErr != nil {
		return runErr
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return err
	}
	if info.Size() > 64*1024*1024 {
		return fmt.Errorf("样式产物超过 64 MiB")
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}
