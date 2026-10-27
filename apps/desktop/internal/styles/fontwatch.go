package styles

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Poll metadata only, not font bytes. These are the same system/user locations
// scanned by SmartSub. New per-user Fonts directories are detected as well.
func fontDirectories() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return []string{filepath.Join(os.Getenv("WINDIR"), "Fonts"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")}
	case "darwin":
		return []string{"/System/Library/Fonts", "/Library/Fonts", filepath.Join(home, "Library", "Fonts")}
	default:
		return []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(home, ".fonts"), filepath.Join(home, ".local", "share", "fonts")}
	}
}

func fontSignature(roots []string) [32]byte {
	hash := sha256.New()
	for _, root := range roots {
		// Unavailable directories are absent from SmartSub's catalog too.
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" && ext != ".otc" {
				return nil
			}
			info, err := entry.Info()
			if err == nil {
				fmt.Fprintf(hash, "%s\x00%d\x00%d\n", path, info.Size(), info.ModTime().UnixNano())
			}
			return nil
		})
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum
}

func WatchFonts(ctx context.Context, changed func()) {
	roots := fontDirectories()
	last := fontSignature(roots)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next := fontSignature(roots)
			if next != last {
				last = next
				changed()
			}
		}
	}
}
