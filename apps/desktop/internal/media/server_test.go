package media

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewCapabilityRangeAndRevocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "movie.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	url, err := s.Register(path)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("Range", "bytes=2-5")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 206 || string(raw) != "2345" {
		t.Fatal(response.StatusCode, string(raw))
	}
	for _, mode := range []string{"host", "origin", "path", "method"} {
		req, _ := http.NewRequest("GET", url, nil)
		switch mode {
		case "host":
			req.Host = "attacker.invalid"
		case "origin":
			req.Header.Set("Origin", "https://attacker.invalid")
		case "path":
			req.URL.Path += "/../secret"
		case "method":
			req.Method = "POST"
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode < 400 {
			t.Fatal("accepted", mode)
		}
	}
	if _, err = s.Register(""); err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("old token works")
	}
}
