package translator

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

//go:embed profiles/*/SKILL.md
var profileFiles embed.FS

type Profile struct {
	ID     string
	Name   string
	Rules  string
	SHA256 string
	Custom bool
	// References are the subtitle skill's reference files the style adds to
	// the core, as the 3D style adds references/glossary-3d-zh.md.
	References []string
}

// reference is the only form a style may name: a file in the skill's
// references folder, never a path out of it.
var reference = regexp.MustCompile(`^references/[a-z0-9][a-z0-9-]*\.md$`)

// Profiles are declarative SKILL.md documents, never executable plugins.
// Reload for each settings visit and each request; no process-wide cache can
// keep a deleted or edited profile active. Core skill rules are always loaded.
func Profiles(directory string) ([]Profile, error) {
	profiles := map[string]Profile{}
	load := func(fsys fs.FS, custom bool) error {
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			path := entry.Name() + "/SKILL.md"
			info, err := fs.Stat(fsys, path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if info.Size() > 64*1024 {
				return errors.New(locales.ErrProfileSize(entry.Name()))
			}
			raw, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			text := strings.ReplaceAll(string(raw), "\r\n", "\n")
			parts := strings.SplitN(text, "\n---\n", 2)
			if !strings.HasPrefix(text, "---\n") || len(parts) != 2 {
				return errors.New(locales.ErrProfileMetadata(entry.Name()))
			}
			name := ""
			refs := []string{}
			for _, line := range strings.Split(parts[0], "\n") {
				if strings.HasPrefix(line, "name:") {
					name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), "\"'")
				}
				// references: a, b — skill reference files the style adds.
				if strings.HasPrefix(line, "references:") {
					for _, ref := range strings.Split(strings.TrimPrefix(line, "references:"), ",") {
						if ref = strings.TrimSpace(ref); !reference.MatchString(ref) {
							return errors.New(locales.ErrProfileReference(entry.Name(), ref))
						}
						refs = append(refs, strings.TrimSpace(ref))
					}
				}
			}
			if name == "" || strings.TrimSpace(parts[1]) == "" {
				return errors.New(locales.ErrProfileContent(entry.Name()))
			}
			profiles[entry.Name()] = Profile{ID: entry.Name(), Name: name, Rules: parts[1], SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Custom: custom, References: refs}
		}
		return nil
	}
	builtin, _ := fs.Sub(profileFiles, "profiles")
	if err := load(builtin, false); err != nil {
		return nil, err
	}
	if directory != "" {
		if err := load(os.DirFS(directory), true); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	result := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == "general" {
			return true
		}
		if result[j].ID == "general" {
			return false
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func loadProfile(directory, id string) (Profile, error) {
	if id == "" {
		id = "general"
	}
	profiles, err := Profiles(directory)
	if err != nil {
		return Profile{}, err
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, errors.New(locales.ErrProfileMissing(id))
}
