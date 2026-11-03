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
// A broken user style is left out on its own: the built-in styles and the
// other user styles still load, and the error names every one left out.
func Profiles(directory string) ([]Profile, error) {
	profiles, broken := readProfiles(directory)
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
	return result, joinBroken(broken)
}

// readProfiles reads the built-in styles, then the user's; broken holds
// the error of each user style left out, by folder name.
func readProfiles(directory string) (map[string]Profile, map[string]error) {
	profiles, broken := map[string]Profile{}, map[string]error{}
	load := func(fsys fs.FS, custom bool) {
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			if custom && !os.IsNotExist(err) {
				broken[""] = err
			}
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			p, err := readProfile(fsys, entry.Name(), custom)
			if err != nil {
				broken[entry.Name()] = err
				continue
			}
			if p.ID == "" {
				continue
			}
			// A user style never replaces a built-in one of the same ID.
			if _, builtin := profiles[p.ID]; custom && builtin {
				broken[p.ID] = errors.New(locales.ErrProfileReserved(p.ID))
				continue
			}
			profiles[p.ID] = p
		}
	}
	builtin, _ := fs.Sub(profileFiles, "profiles")
	load(builtin, false)
	if directory != "" {
		load(os.DirFS(directory), true)
	}
	return profiles, broken
}

// readProfile reads one style folder; a folder without SKILL.md is no style.
// The metadata may be saved with a BOM, indented, quoted, and may list
// references on one line or as "- item" lines.
func readProfile(fsys fs.FS, id string, custom bool) (Profile, error) {
	path := id + "/SKILL.md"
	info, err := fs.Stat(fsys, path)
	if os.IsNotExist(err) {
		return Profile{}, nil
	}
	if err != nil {
		return Profile{}, err
	}
	if info.Size() > 64*1024 {
		return Profile{}, errors.New(locales.ErrProfileSize(id))
	}
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return Profile{}, err
	}
	text := strings.TrimPrefix(strings.ReplaceAll(string(raw), "\r\n", "\n"), string(rune(0xFEFF)))
	parts := strings.SplitN(text, "\n---\n", 2)
	if !strings.HasPrefix(text, "---\n") || len(parts) != 2 {
		return Profile{}, errors.New(locales.ErrProfileMetadata(id))
	}
	name := ""
	refs := []string{}
	addRef := func(ref string) error {
		ref = strings.Trim(strings.TrimSpace(ref), "\"'")
		if ref == "" {
			return nil
		}
		if !reference.MatchString(ref) {
			return errors.New(locales.ErrProfileReference(id, ref))
		}
		refs = append(refs, ref)
		return nil
	}
	inRefs := false
	for _, line := range strings.Split(strings.TrimPrefix(parts[0], "---\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- ") && inRefs:
			if err := addRef(strings.TrimPrefix(trimmed, "- ")); err != nil {
				return Profile{}, err
			}
			continue
		case strings.HasPrefix(trimmed, "name:"):
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "name:")), "\"'")
		case strings.HasPrefix(trimmed, "references:"):
			// references: a, b — or [a, b], or a list on the lines below.
			value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "references:")), "[]")
			for _, ref := range strings.Split(value, ",") {
				if err := addRef(ref); err != nil {
					return Profile{}, err
				}
			}
			inRefs = true
			continue
		}
		if trimmed != "" {
			inRefs = false
		}
	}
	if name == "" || strings.TrimSpace(parts[1]) == "" {
		return Profile{}, errors.New(locales.ErrProfileContent(id))
	}
	return Profile{ID: id, Name: name, Rules: parts[1], SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Custom: custom, References: refs}, nil
}

// joinBroken is one error naming every user style left out, in name order.
func joinBroken(broken map[string]error) error {
	names := make([]string, 0, len(broken))
	for name := range broken {
		names = append(names, name)
	}
	sort.Strings(names)
	errs := make([]error, 0, len(names))
	for _, name := range names {
		errs = append(errs, broken[name])
	}
	return errors.Join(errs...)
}

func loadProfile(directory, id string) (Profile, error) {
	if id == "" {
		id = "general"
	}
	profiles, broken := readProfiles(directory)
	if p, ok := profiles[id]; ok {
		return p, nil
	}
	if err := broken[id]; err != nil {
		return Profile{}, err
	}
	return Profile{}, errors.New(locales.ErrProfileMissing(id))
}
