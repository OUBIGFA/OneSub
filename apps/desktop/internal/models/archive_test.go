package models

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A real compressed archive exercises the published-model download path.
func TestSpeakerModelArchiveHashAndMember(t *testing.T) {
	raw, _ := base64.StdEncoding.DecodeString("QlpoOTFBWSZTWXQlJC4AAHF7gMmAAAJAAe8AQABnJ55ACAggAFQyo00aNNGTRoG1PUEkk9Q0AAAB97SQQg2dCEWdXRL6qJ0CGBiOlridhGUCNgzIWTi0pzUTzG/bQvWCIPKqv16yJIH4u5IpwoSDoSkhcA==")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
	defer server.Close()
	spec := File{Name: "model.onnx", Size: 5, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("model"))), URL: server.URL, ArchiveMember: "fixed/model.onnx", ArchiveSize: int64(len(raw)), ArchiveSHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
	dir := t.TempDir()
	if err := Download(context.Background(), server.Client(), Model{Files: []File{spec}}, dir, func(Progress) {}); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(filepath.Join(dir, "model.onnx"))
	if err != nil || string(value) != "model" {
		t.Fatal(string(value), err)
	}
	spec.ArchiveMember = "missing.onnx"
	if err := Download(context.Background(), server.Client(), Model{Files: []File{spec}}, t.TempDir(), func(Progress) {}); err == nil {
		t.Fatal("accepted missing member")
	}
	spec.ArchiveSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("different")))
	if err := Download(context.Background(), server.Client(), Model{Files: []File{spec}}, t.TempDir(), func(Progress) {}); err == nil {
		t.Fatal("accepted changed archive")
	}
}
