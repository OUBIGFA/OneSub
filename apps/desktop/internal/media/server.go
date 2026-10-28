package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Server serves one registered preview, never arbitrary paths from a URL. A new
// registration revokes the previous unguessable capability, including on reopen.
type Server struct {
	mu                sync.RWMutex
	server            *http.Server
	host, token, path string
	fonts             map[string]string
}

func NewServer() (*Server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{host: listener.Addr().String()}
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go s.server.Serve(listener)
	return s, nil
}
func (s *Server) Register(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token, s.path = "", ""
	s.fonts = nil
	if path == "" {
		return "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New(locales.ErrPreviewNotFile())
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", err
	}
	s.token, s.path = hex.EncodeToString(raw), path
	return "http://" + s.host + "/media/" + s.token, nil
}

// RegisterFonts grants access only to the exact SmartSub-resolved files. Their
// hashes must still match, and replacing a document revokes older font URLs.
func (s *Server) RegisterFonts(ctx context.Context, document styles.Document) ([]string, error) {
	paths := map[string]string{}
	urls := []string{}
	for _, font := range document.Fonts {
		sum, err := hashFile(ctx, font.Path)
		if err != nil {
			return nil, err
		}
		if sum != font.SHA256 {
			return nil, errors.New(locales.ErrPreviewFontChanged())
		}
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return nil, err
		}
		route := "/fonts/" + hex.EncodeToString(raw)
		paths[route] = font.Path
		urls = append(urls, "http://"+s.host+route)
	}
	s.mu.Lock()
	s.fonts = paths
	s.mu.Unlock()
	return urls, nil
}

func (s *Server) Close() error { return s.server.Close() }
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	path, token := s.path, s.token
	fontPath := s.fonts[r.URL.Path]
	s.mu.RUnlock()
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Host != s.host || (fontPath == "" && (token == "" || r.URL.Path != "/media/"+token)) || r.URL.RawQuery != "" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	// Requests without Origin occur during normal HTML media playback. The token,
	// exact host and exact route remain mandatory; CORS is not authorization.
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://mygo.localhost" && origin != "https://mygo.localhost" && origin != "mygo://app" && origin != "http://localhost:5173" {
		http.Error(w, "Forbidden", 403)
		return
	}
	if fontPath != "" {
		path = fontPath
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return
	}
	if strings.Contains(r.Header.Get("Range"), ",") {
		http.Error(w, "Multiple ranges are not supported", 416)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "Media unavailable", 404)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "Media unavailable", 404)
		return
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}
