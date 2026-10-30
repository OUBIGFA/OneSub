// Package subtitle exchanges bounded artifacts with the self-contained libse worker.
package subtitle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type Word struct {
	Text    string `json:"text"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
}

type SourceRange struct {
	FirstAtom string `json:"firstAtom"`
	LastAtom  string `json:"lastAtom"`
	Reason    string `json:"reason"`
}

type Cue struct {
	ID                string       `json:"id"`
	Position          int          `json:"position"`
	StartMs           int64        `json:"startMs"`
	EndMs             int64        `json:"endMs"`
	Text              string       `json:"text"`
	OriginalText      string       `json:"originalText"`
	Translated        bool         `json:"translated,omitempty"`
	TranslationSource string       `json:"translationSource,omitempty"`
	ReadOnly          bool         `json:"readOnly"`
	TextLocked        bool         `json:"textLocked"`
	TimeLocked        bool         `json:"timeLocked"`
	Words             []Word       `json:"words,omitempty"`
	SourceCueIDs      []string     `json:"sourceCueIds,omitempty"`
	SourceRange       *SourceRange `json:"sourceRange,omitempty"`
	TimingQuality     string       `json:"timingQuality,omitempty"`
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
		err = errors.New(locales.ErrSubtitleEmpty())
	}
	return doc, err
}

func (c Client) Write(ctx context.Context, doc Document) ([]byte, error) {
	return c.documentOperation(ctx, "write", doc)
}

// SelectLines makes an explicit export projection through libse. It preserves
// the source document and never applies extraction rules to ordinary saves.
func (c Client) SelectLines(ctx context.Context, doc Document, line string) (Document, error) {
	if line == "" || line == "all" {
		return doc, nil
	}
	data, err := c.documentOperation(ctx, "select-lines", struct {
		Document Document `json:"document"`
		Line     string   `json:"line"`
	}{doc, line})
	var selected Document
	if err == nil {
		err = json.Unmarshal(data, &selected)
	}
	return selected, err
}

func (c Client) documentOperation(ctx context.Context, operation string, document any) ([]byte, error) {
	dir, err := os.MkdirTemp(c.Scratch, "serialize-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // Only this invocation's generated artifacts.
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	input := filepath.Join(dir, "document.json")
	if err = os.WriteFile(input, data, 0600); err != nil {
		return nil, err
	}
	return c.run(ctx, operation, input)
}

// boundedBuffer prevents a broken helper from allocating unbounded stdout/stderr.
type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New(locales.ErrWorkerOutput())
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
		return nil, apperr.Wrap(locales.ErrSubtitleWorker(fmt.Sprint(runErr)), runErr)
	}
	if !reply.OK {
		return nil, errors.New(locales.ErrSubtitle(workerMessage(reply.Error)))
	}
	if runErr != nil {
		return nil, apperr.Wrap(locales.ErrSubtitleExit(runErr.Error()), runErr)
	}
	f, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024*1024+1))
	if len(data) > 64*1024*1024 {
		return nil, errors.New(locales.ErrSubtitleResultSize())
	}
	return data, err
}

// workerMessage says the subtitle worker's stable E_ codes in the app's
// language. libse and system messages pass unchanged.
func workerMessage(message string) string {
	switch message {
	case "E_FORMAT":
		return locales.ErrSubtitleFormat()
	case "E_ASS_MAPPING", "E_VTT_MAPPING":
		return locales.ErrSubtitleMapping()
	case "E_TIMING":
		return locales.ErrSubtitleTiming()
	case "E_EMPTY_CUE":
		return locales.ErrSubtitleEmptyCue()
	case "E_NO_CUES":
		return locales.ErrSubtitleNoCues()
	case "E_PRESERVE_ORDER":
		return locales.ErrSubtitlePreserveOrder()
	case "E_ASS_TIMING":
		return locales.ErrSubtitleAssTiming()
	case "E_ASS_DRAWING":
		return locales.ErrSubtitleAssDrawing()
	case "E_ASS_TAGS":
		return locales.ErrSubtitleAssTags()
	case "E_ASS_NEEDS_STYLE":
		return locales.ErrSubtitleAssStyle()
	case "E_VTT_BLANK":
		return locales.ErrSubtitleVttBlank()
	case "E_ASS_FORMAT":
		return locales.ErrSubtitleAssFormat()
	case "E_LINE_SELECTION":
		return locales.ErrSubtitleLineSelection()
	case "E_EMPTY_LINE":
		return locales.ErrSubtitleSelectedLineEmpty()
	case "E_CONTROL_SIZE", "E_INPUT_SIZE":
		return locales.ErrSubtitleInputSize()
	case "E_PROTOCOL":
		return locales.ErrSubtitleProtocol()
	case "E_OUTPUT_EXISTS":
		return locales.ErrSubtitleOutputExists()
	}
	if count, ok := strings.CutPrefix(message, "E_PARSE:"); ok {
		return locales.ErrSubtitleParse(count)
	}
	return message
}
