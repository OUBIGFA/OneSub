package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Business packages are shared by the native workbench and the MCP server.
// Keep view construction out of that boundary so backend work needs no window.
func TestBusinessPackagesDoNotImportViews(t *testing.T) {
	err := filepath.WalkDir("internal", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			dependency, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if dependency == "github.com/egoist/mygo/ui" || strings.HasPrefix(dependency, "github.com/egoist/mygo/ui/") || strings.HasPrefix(dependency, "github.com/lobehub/lobe-mygo/") {
				t.Errorf("%s imports a view dependency: %s", path, dependency)
			}
			// Encoder choices are a leaf policy, independent of persistence,
			// process execution, localization and the app's composition root.
			if file.Name.Name == "videoencoding" && strings.Contains(strings.SplitN(dependency, "/", 2)[0], ".") {
				t.Errorf("encoding policy must remain independent: %s", dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
