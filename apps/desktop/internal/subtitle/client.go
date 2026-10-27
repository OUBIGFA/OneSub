// Package subtitle exchanges bounded artifacts with the self-contained libse worker.
package subtitle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Cue struct {
	ID                string `json:"id"`
	Position          int    `json:"position"`
	StartMs           int64  `json:"startMs"`
	EndMs             int64  `json:"endMs"`
	Text              string `json:"text"`
	OriginalText      string `json:"originalText"`
	Translated        bool   `json:"translated,omitempty"`
	TranslationSource string `json:"translationSource,omitempty"`
	ReadOnly          bool   `json:"readOnly"`
	TextLocked        bool   `json:"textLocked"`
	TimeLocked        bool   `json:"timeLocked"`
}

type Document struct {
	Format       string `json:"format"`
	TargetFormat string `json:"targetFormat"`
	Original     string `json:"original"`
	Cues         []Cue  `json:"cues"`
}

// Client runs one request per process. Scratch is owned by this application,
// never the source directory or the final export destination.
type Client struct{ Executable, Scratch string }

func (c Client) Read(ctx context.Context, path string) (Document, error) {
	data, err := c.run(ctx, "read", path)
	var doc Document
	if err == nil {
		err = json.Unmarshal(data, &doc)
	}
	if err == nil && len(doc.Cues) == 0 {
		err = errors.New("字幕工作进程没有返回可编辑字幕")
	}
	return doc, err
}

func (c Client) Write(ctx context.Context, doc Document) ([]byte, error) {
	dir, err := os.MkdirTemp(c.Scratch, "serialize-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // Only this invocation's generated artifacts.
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	input := filepath.Join(dir, "document.json")
	if err = os.WriteFile(input, data, 0600); err != nil {
		return nil, err
	}
	return c.run(ctx, "write", input)
}

// boundedBuffer prevents a broken helper from allocating unbounded stdout/stderr.
type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("工作进程输出超过限制")
	}
	return b.Buffer.Write(p)
}

func (c Client) run(ctx context.Context, operation, input string) ([]byte, error) {
	dir, err := os.MkdirTemp(c.Scratch, "subtitle-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "result")
	id := filepath.Base(dir)
	request, err := json.Marshal(map[string]any{"protocol": 1, "requestId": id, "operation": operation, "inputPath": input, "outputPath": output})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := jobs.CommandContext(ctx, c.Executable)
	cmd.Stdin = bytes.NewReader(request)
	stdout := &boundedBuffer{limit: 1024 * 1024}
	stderr := &boundedBuffer{limit: 64 * 1024}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	runErr := cmd.Run()
	var reply struct {
		Protocol  int
		RequestID string
		OK        bool
		Error     string
	}
	if err = json.Unmarshal(stdout.Bytes(), &reply); err != nil || reply.Protocol != 1 || reply.RequestID != id {
		return nil, fmt.Errorf("字幕工作进程协议无效：%v", runErr)
	}
	if !reply.OK {
		return nil, fmt.Errorf("字幕处理失败：%s", reply.Error)
	}
	if runErr != nil {
		return nil, fmt.Errorf("字幕工作进程未正常退出：%w", runErr)
	}
	f, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024*1024+1))
	if len(data) > 64*1024*1024 {
		return nil, errors.New("字幕结果超过 64 MiB")
	}
	return data, err
}
