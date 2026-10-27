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

func TestCatalogHasAllEightChoices(t *testing.T) {
	c := Catalog()
	if len(c) != 8 {
		t.Fatal(len(c))
	}
	for _, m := range c {
		if len(m.Files) == 0 || m.Size() == 0 {
			t.Fatal(m)
		}
		for _, f := range m.Files {
			if len(f.SHA256) != 64 || !strings.Contains(f.URL, "/resolve/") || strings.Contains(f.URL, "/master/") {
				t.Fatal(f)
			}
		}
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
