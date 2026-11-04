package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCatalogHasOneAsrAndSherpaChoices(t *testing.T) {
	c := Catalog()
	if len(c) != 13 {
		t.Fatal(len(c))
	}
	ids := map[string]bool{}
	for _, m := range c {
		if len(m.Files) == 0 || m.Size() == 0 || ids[m.ID] {
			t.Fatal(m)
		}
		ids[m.ID] = true
		if (m.Kind == "asr" && m.Sherpa()) != (len(m.Languages) > 0) || (m.Sherpa() && m.Kind != "asr" && m.Kind != "lid") {
			t.Fatal(m)
		}
		for _, f := range m.Files {
			if len(f.SHA256) != 64 || !strings.Contains(f.URL, "/resolve/") || strings.Contains(f.URL, "/master/") {
				t.Fatal(f)
			}
		}
	}
}
func TestModelPartsExcludeExternalWeights(t *testing.T) {
	for _, id := range []string{"sherpa-parakeet-tdt-0.6b-v3-int8", "sherpa-parakeet-tdt-0.6b-v3-fp32"} {
		m, err := ByID(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"encoder", "decoder", "joiner"} {
			if name := m.Part(part); !strings.HasSuffix(name, ".onnx") {
				t.Fatal(id, part, name)
			}
		}
		if m.Part("tokens") != "tokens.txt" || m.Part("missing") != "" {
			t.Fatal(id, "incorrect catalog part")
		}
	}
	// External weights may precede the executable graph in the catalog.
	m := Model{Files: []File{{Name: "encoder.weights"}, {Name: "encoder.onnx.data"}, {Name: "encoder.onnx"}}}
	if m.Part("encoder") != "encoder.onnx" {
		t.Fatal("external data was used as a graph", m.Part("encoder"))
	}
}

func TestRoutesCoverEveryGroup(t *testing.T) {
	for _, group := range RouteGroups {
		model, err := ByID(DefaultRoute(group))
		if err != nil || !model.GroupSupported(group) {
			t.Fatal(group, model, err)
		}
	}
	if RouteGroup("nl") != "eu" || RouteGroup("yue") != "yue" || RouteGroup("th") != "" {
		t.Fatal("language groups")
	}
	if lid, err := ByID(LanguageDetector); err != nil || lid.Kind != "lid" {
		t.Fatal(lid, err)
	}
}

// FP32's encoder graph loads a separate weights file. Verify that complete
// asset set with the same production verifier used by downloads and imports.
func TestVerifyParakeetFP32RealAssets(t *testing.T) {
	root := os.Getenv("ONESUB_SHERPA_TEST_MODELS")
	if root == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS to verified model folders")
	}
	model, err := ByID("sherpa-parakeet-tdt-0.6b-v3-fp32")
	if err != nil {
		t.Fatal(err)
	}
	if err = Verify(context.Background(), model, filepath.Join(root, model.ID)); err != nil {
		t.Fatal("FP32 model files failed catalog verification", err)
	}
}

func TestFilesPresentRequiresCompleteCatalogAssets(t *testing.T) {
	model := Model{Files: []File{{Name: "encoder.onnx", Size: 4}, {Name: "encoder.weights", Size: 6}}}
	dir := t.TempDir()
	if model.FilesPresent(dir) || (Model{}).FilesPresent(dir) {
		t.Fatal("empty directory or catalog was listed as downloaded")
	}
	os.WriteFile(filepath.Join(dir, "encoder.onnx"), []byte("onnx"), 0600)
	os.WriteFile(filepath.Join(dir, "encoder.weights.partial"), []byte("weight"), 0600)
	if model.FilesPresent(dir) {
		t.Fatal("partial external weights were accepted")
	}
	os.WriteFile(filepath.Join(dir, "encoder.weights"), []byte("short"), 0600)
	if model.FilesPresent(dir) {
		t.Fatal("wrong-size weights were accepted")
	}
	os.WriteFile(filepath.Join(dir, "encoder.weights"), []byte("weight"), 0600)
	if !model.FilesPresent(dir) {
		t.Fatal("complete local assets were excluded")
	}
	os.Remove(filepath.Join(dir, "encoder.weights"))
	os.Mkdir(filepath.Join(dir, "encoder.weights"), 0700)
	if model.FilesPresent(dir) {
		t.Fatal("directory masquerading as a model file was accepted")
	}
}

func TestDownloadResumeAndHashValidation(t *testing.T) {
	data := []byte("real fixture bytes")
	hash := sha256.Sum256(data)
	requested := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.Header.Get("Range")
		http.ServeContent(w, r, "weights", time.Time{}, strings.NewReader(string(data)))
	}))
	defer server.Close()
	m := Model{ID: "test", Files: []File{{Name: "weights", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), URL: server.URL}}}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "weights.partial"), data[:4], 0600)
	if err := Download(context.Background(), server.Client(), m, dir, func(Progress) {}); err != nil {
		t.Fatal(err)
	}
	if requested != "bytes=4-" {
		t.Fatal(requested)
	}
	if err := Verify(context.Background(), m, dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "weights"), []byte(strings.Repeat("x", len(data))), 0600)
	if err := Verify(context.Background(), m, dir); err == nil {
		t.Fatal("same-size corruption accepted")
	}
}
