// Package models consumes OneAsr's exact model IDs, file sizes and hashes,
// and the pinned sherpa-onnx models of the other recognition engines.
package models

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

//go:embed catalog.json
var catalogJSON []byte

// catalog_sherpa.json lists the sherpa-onnx models. It stays apart from
// catalog.json, which tools/import_oneasr_catalog.py regenerates.
//
//go:embed catalog_sherpa.json
var sherpaJSON []byte

type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Engine is the sherpa-onnx model family that runs an ASR model; it is
	// empty for OneAsr's own models. Languages are the source languages a
	// sherpa-onnx model recognizes.
	Engine    string   `json:"engine,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Files     []File   `json:"files"`
}

// Sherpa engines.
const (
	EngineSenseVoice = "sense-voice"
	EngineParaformer = "paraformer"
	EngineTransducer = "transducer"
	EngineWhisperLID = "whisper-lid"
)

// Automatic recognition: the language detector, the model each language
// group uses unless the user chooses another, and the fallback model for
// speech whose language stays unclear. The groups are the source
// languages with a model of their own; "eu" is every European language
// that Parakeet-TDT recognizes besides English.
const (
	LanguageDetector = "sherpa-whisper-tiny-int8"
	FallbackModel    = "sherpa-sense-voice-small-int8"
)

// RouteGroups are the language groups of automatic recognition, in the
// order the settings show them.
var RouteGroups = []string{"zh", "yue", "ja", "ko", "en", "eu"}

// RouteGroup is the group of a detected language: the language itself,
// "eu" for the other European languages of Parakeet-TDT, or "" for one
// that no group covers.
func RouteGroup(language string) string {
	switch language {
	case "zh", "yue", "ja", "ko", "en":
		return language
	}
	if parakeet, err := ByID("sherpa-parakeet-tdt-0.6b-v3-int8"); err == nil && slices.Contains(parakeet.Languages, language) {
		return "eu"
	}
	return ""
}

// DefaultRoute is the model a group uses by default: SenseVoiceSmall for
// Chinese, Cantonese, Japanese and Korean, Parakeet-TDT for English and
// the European languages.
func DefaultRoute(group string) string {
	switch group {
	case "en", "eu":
		return "sherpa-parakeet-tdt-0.6b-v3-int8"
	}
	return FallbackModel
}

// GroupSupported reports whether model recognizes the languages of group.
func (m Model) GroupSupported(group string) bool {
	if m.Kind != "asr" {
		return false
	}
	if group == "eu" {
		return !m.Sherpa() || m.Engine == EngineTransducer
	}
	return m.Supports(group)
}

// Sherpa reports whether sherpa-onnx, not OneAsr, runs the model. Such a
// model needs no forced aligner: it times its own tokens.
func (m Model) Sherpa() bool { return m.Engine != "" }

// oneAsrLanguages are the languages OneAsr v1.3.0 recognizes with its
// forced aligner; it has no automatic detection.
var oneAsrLanguages = []string{"zh", "en", "yue", "ja", "ko", "de", "fr", "es", "it", "pt", "ru"}

// Supports reports whether the model recognizes language.
func (m Model) Supports(language string) bool {
	if !m.Sherpa() {
		return slices.Contains(oneAsrLanguages, language)
	}
	return slices.Contains(m.Languages, language)
}

func Catalog() []Model {
	models := []Model{}
	for _, data := range [][]byte{catalogJSON, sherpaJSON} {
		var c struct {
			Models []Model `json:"models"`
		}
		if err := json.Unmarshal(data, &c); err != nil {
			panic(err)
		}
		models = append(models, c.Models...)
	}
	return models
}
func ByID(id string) (Model, error) {
	for _, m := range Catalog() {
		if m.ID == id {
			return m, nil
		}
	}
	return Model{}, errors.New(locales.ErrModelUnknown(id))
}
func (m Model) Size() int64 {
	var size int64
	for _, f := range m.Files {
		size += f.Size
	}
	return size
}
func Verify(ctx context.Context, m Model, dir string) error {
	for _, f := range m.Files {
		if err := verifyFile(ctx, filepath.Join(dir, f.Name), f); err != nil {
			return err
		}
	}
	return nil
}
func verifyFile(ctx context.Context, path string, expected File) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New(locales.ErrModelMissing(expected.Name))
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != expected.Size {
		return errors.New(locales.ErrModelSize(expected.Name))
	}
	hash := sha256.New()
	buf := make([]byte, 1024*1024)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := file.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return errors.New(locales.ErrModelHash(expected.Name))
	}
	return nil
}

// Download stages; the interface says them in its language.
const (
	StageDownload = "download"
	StageVerify   = "verify"
	StageDone     = "done"
)

type Progress struct {
	Done, Total int64
	Stage       string
}

// Download resumes only the requested model's partials. A file becomes visible
// under its final name only after exact length and SHA-256 validation.
func Download(ctx context.Context, client *http.Client, m Model, dir string, progress func(Progress)) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	var completed int64
	for _, spec := range m.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		final := filepath.Join(dir, spec.Name)
		if err := verifyFile(ctx, final, spec); err == nil {
			completed += spec.Size
			progress(Progress{completed, m.Size(), StageDone})
			continue
		}
		partial := final + ".partial"
		var offset int64
		if info, err := os.Stat(partial); err == nil {
			offset = info.Size()
		}
		if offset > spec.Size {
			return errors.New(locales.ErrModelPartial(spec.Name))
		}
		if offset < spec.Size {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
			if err != nil {
				return err
			}
			if offset > 0 {
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			}
			response, err := client.Do(req)
			if err != nil {
				return err
			}
			if response.StatusCode != 200 && response.StatusCode != 206 {
				response.Body.Close()
				return errors.New(locales.ErrModelHttp(response.StatusCode))
			}
			if response.StatusCode == 200 {
				offset = 0
			} else if !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(offset, 10)+"-") {
				response.Body.Close()
				return errors.New(locales.ErrModelRange())
			}
			flag := os.O_CREATE | os.O_WRONLY
			if offset == 0 {
				flag |= os.O_TRUNC
			} else {
				flag |= os.O_APPEND
			}
			file, err := os.OpenFile(partial, flag, 0600)
			if err != nil {
				response.Body.Close()
				return err
			}
			buf := make([]byte, 256*1024)
			last := time.Time{}
			for {
				n, readErr := response.Body.Read(buf)
				if offset+int64(n) > spec.Size {
					err = errors.New(locales.ErrModelOversize())
					break
				}
				if n > 0 {
					var written int
					written, err = file.Write(buf[:n])
					offset += int64(written)
					if err != nil {
						break
					}
				}
				if time.Since(last) > 200*time.Millisecond {
					progress(Progress{completed + offset, m.Size(), StageDownload})
					last = time.Now()
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					err = readErr
					break
				}
			}
			if syncErr := file.Sync(); err == nil {
				err = syncErr
			}
			closeErr := file.Close()
			response.Body.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
		}
		progress(Progress{completed + offset, m.Size(), StageVerify})
		if err := verifyFile(ctx, partial, spec); err != nil {
			return err
		}
		if err := os.Rename(partial, final); err != nil {
			return err
		}
		completed += spec.Size
	}
	progress(Progress{m.Size(), m.Size(), StageDone})
	return nil
}
