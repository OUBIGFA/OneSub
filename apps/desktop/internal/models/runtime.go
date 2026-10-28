package models

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// RuntimeRoot exposes selected directories under OneAsr's documented catalog
// layout, without copying multi-GB weights or changing users' model folders.
// The original CLI/core remains the loader. Links are removed before the owned
// empty layout, so cleanup never recursively visits external model directories.
func RuntimeRoot(ctx context.Context, scratch string, paths map[string]string) (string, func() error, error) {
	root, err := os.MkdirTemp(scratch, "oneasr-models-")
	if err != nil {
		return "", nil, err
	}
	links := []string{}
	cleanup := func() error {
		for _, link := range links {
			if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return os.RemoveAll(root)
	}
	if err = os.Mkdir(filepath.Join(root, "models"), 0700); err != nil {
		cleanup()
		return "", nil, err
	}
	for id, path := range paths {
		if _, err = ByID(id); err != nil {
			cleanup()
			return "", nil, err
		}
		target, err := filepath.Abs(path)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		info, err := os.Stat(target)
		if err != nil || !info.IsDir() {
			cleanup()
			return "", nil, errors.New(locales.ErrModelFolder(id))
		}
		link := filepath.Join(root, "models", id)
		if runtime.GOOS == "windows" {
			// Encoded, single-quoted literals handle Chinese, spaces, apostrophes
			// and shell metacharacters without evaluating any path as code.
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
			script := "$ErrorActionPreference='Stop'; New-Item -ItemType Junction -Path " + quote(link) + " -Target " + quote(target) + " | Out-Null"
			units := utf16.Encode([]rune(script))
			raw := make([]byte, len(units)*2)
			for i, u := range units {
				binary.LittleEndian.PutUint16(raw[i*2:], u)
			}
			err = jobs.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw)).Run()
		} else {
			err = os.Symlink(target, link)
		}
		if err != nil {
			cleanup()
			return "", nil, apperr.Wrap(locales.ErrModelLink(err.Error()), err)
		}
		links = append(links, link)
		actual, err := os.Stat(link)
		if err != nil || !os.SameFile(info, actual) {
			cleanup()
			return "", nil, errors.New(locales.ErrModelLinkCheck(id))
		}
	}
	return root, cleanup, nil
}
