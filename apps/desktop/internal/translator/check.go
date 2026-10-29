package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Findings is the unchanged subtitle skill's complete-file report. Warnings
// remain visible and are not evidence of semantic or acoustic correctness.
type Findings struct {
	Format   string   `json:"format"`
	Language string   `json:"lang"`
	Blocks   int      `json:"blocks"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Notes    []string `json:"notes"`
}

type Checker struct {
	Executable, SkillRoot, Scratch string
	Subtitle                       subtitle.Client
}

// Check serializes one whole document with libse before invoking the original
// checker. We do not concatenate batch fragments or normalize strict ASS with
// an assembler: the original document's metadata remains authoritative.
// strict compares block by block, for a timeline kept as it was; a rebuilt
// one is checked against the source's speech, as the skill checks it.
func (c Checker) Check(ctx context.Context, source, candidate subtitle.Document, language string, strict bool) (Findings, error) {
	var result Findings
	if source.Format != candidate.Format {
		return result, errors.New(locales.ErrCheckFormatChange())
	}
	switch source.Format {
	case "srt", "vtt", "ass", "ssa":
	default:
		return result, errors.New(locales.ErrCheckFormat())
	}
	dir, err := os.MkdirTemp(c.Scratch, "skill-check-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(dir)
	source.TargetFormat, candidate.TargetFormat = source.Format, source.Format
	for name, document := range map[string]subtitle.Document{"source": source, "candidate": candidate} {
		data, err := c.Subtitle.Write(ctx, document)
		if err != nil {
			return result, apperr.Wrap(locales.ErrCheckSubtitle(err.Error()), err)
		}
		if err = os.WriteFile(filepath.Join(dir, name+"."+source.Format), data, 0600); err != nil {
			return result, err
		}
	}
	id := filepath.Base(dir)
	output := filepath.Join(dir, "findings.json")
	request, err := json.Marshal(map[string]any{
		"protocol": 1, "requestId": id, "operation": "check",
		"inputPath":  filepath.Join(dir, "candidate."+source.Format),
		"sourcePath": filepath.Join(dir, "source."+source.Format),
		"outputPath": output, "language": language, "strict": strict,
		"languageConfig": filepath.Join(c.SkillRoot, "config", "language_profiles.json"),
	})
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.Command(c.Executable)
	cmd.Stdin = bytes.NewReader(request)
	var control strings.Builder
	runErr := jobs.Run(ctx, cmd, func(line string) {
		if control.Len() < 1024*1024 {
			control.WriteString(line)
			control.WriteByte('\n')
		}
	})
	var reply struct {
		Protocol  int
		RequestID string
		OK        bool
		Error     string
	}
	if err = json.Unmarshal([]byte(control.String()), &reply); err != nil || reply.Protocol != 1 || reply.RequestID != id {
		return result, apperr.Wrap(locales.ErrCheckWorker(fmt.Sprint(runErr)), runErr)
	}
	if !reply.OK {
		return result, errors.New(locales.ErrCheck(checkerMessage(reply.Error)))
	}
	if runErr != nil {
		return result, apperr.Wrap(locales.ErrCheckExit(runErr.Error()), runErr)
	}
	file, err := os.Open(output)
	if err != nil {
		return result, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	if err != nil {
		return result, err
	}
	if len(raw) > 4*1024*1024 {
		return result, errors.New(locales.ErrCheckReportSize())
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, errors.New(locales.ErrCheckReport())
	}
	if result.Blocks != len(candidate.Cues) {
		return result, errors.New(locales.ErrCheckCount())
	}
	return result, nil
}

// checkerMessage says the skill checker worker's stable E_ codes in the
// app's language; the checker's own findings are reported unchanged.
func checkerMessage(message string) string {
	switch message {
	case "E_CONTROL_SIZE", "E_PROTOCOL":
		return locales.ErrCheckProtocol()
	case "E_INPUT":
		return locales.ErrCheckInput()
	case "E_RESULT":
		return locales.ErrCheckResult()
	}
	return message
}
