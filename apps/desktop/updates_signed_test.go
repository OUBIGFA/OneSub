package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/egoist/mygo"
)

// The update source serves real signed archives over a real loopback HTTP
// connection. Only its host differs from a published GitHub release.
type updateTestSource struct{ url string }

func (s updateTestSource) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	return s.Open(ctx, s.url+"/update.json")
}

func (s updateTestSource) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return resp.Body, nil
}

func TestSignedUpdateBuildOrderingAndVerification(t *testing.T) {
	keyFile := os.Getenv("ONESUB_UPDATE_TEST_KEY_FILE")
	if keyFile == "" {
		t.Skip("build/test-updates.ps1 runs this test in an isolated signed executable")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(exe)
	if root != os.Getenv("ONESUB_UPDATE_TEST_ROOT") || !mygo.Updater.Enabled() {
		t.Fatal("signed installation test must run in its isolated writable directory")
	}
	keyText, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(keyText)))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		t.Fatal("invalid temporary signing key", err)
	}
	const proofName, newContent = "onesub-update-proof.txt", "new signed build"
	proof := filepath.Join(root, proofName)
	if err := os.WriteFile(proof, []byte("old build"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(proof) })
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: proofName, Mode: 0600, Size: int64(len(newContent))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(newContent)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), sum[:]))
	var mu sync.Mutex
	version, sig, payload := "0.0.1.20261105000001", signature, bytes.Clone(archive.Bytes())
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/update.json" {
			json.NewEncoder(w).Encode(map[string]any{"version": version, "url": server.URL + "/app.tar.gz", "size": archive.Len(), "signature": sig})
		} else {
			w.Write(payload)
		}
	}))
	defer server.Close()
	mygo.Updater.SetSource(updateTestSource{server.URL})
	defer mygo.Updater.SetSource(nil)
	previous := mygo.App.Version()
	mygo.App.SetVersion(orderedUpdateVersion("0.0.1", "20261105000001"))
	defer mygo.App.SetVersion(previous)
	ctx := context.Background()
	for _, noUpdate := range []string{"0.0.1", "0.0.1.20261105000000", "0.0.1.20261105000001"} {
		mu.Lock()
		version = noUpdate
		mu.Unlock()
		up, err := mygo.Updater.Check(ctx)
		if err != nil || up != nil {
			t.Fatal("the running build accepted an equal or older update", noUpdate, up, err)
		}
	}
	mu.Lock()
	version, sig = "0.0.1.20261105000002", base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	mu.Unlock()
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil || up.Version != version {
		t.Fatal("a newer build of the same product was not found", up, err)
	}
	if err := up.Install(ctx, nil); err == nil {
		t.Fatal("an invalid signature was installed")
	}
	if data, _ := os.ReadFile(proof); string(data) != "old build" {
		t.Fatal("a rejected signature changed installed files")
	}
	mu.Lock()
	sig = signature
	payload[0] ^= 1
	mu.Unlock()
	up, err = mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		t.Fatal(err)
	}
	if err := up.Install(ctx, nil); err == nil {
		t.Fatal("a tampered archive was installed")
	}
	if data, _ := os.ReadFile(proof); string(data) != "old build" {
		t.Fatal("a tampered archive changed installed files")
	}
	mu.Lock()
	payload = bytes.Clone(archive.Bytes())
	mu.Unlock()
	var done, total int64
	if err := up.Install(ctx, func(n, size int64) { done, total = n, size }); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(proof); string(data) != newContent {
		t.Fatal("a signed update did not replace the installed file", string(data))
	}
	if done != int64(archive.Len()) || total != done {
		t.Fatal("download progress did not report actual bytes", done, total, archive.Len())
	}
}
