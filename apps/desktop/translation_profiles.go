package main

import (
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func (a *NativeApp) reloadTranslationProfiles() {
	profiles, err := translator.Profiles(filepath.Join(a.dataRoot, "translation-styles"))
	if err != nil {
		a.profilesError = err.Error()
		return
	}
	a.translationProfiles = profiles
	a.profilesError = ""
	if a.settings.TranslationProfile == "" {
		a.settings.TranslationProfile = "general"
		if a.settings.Domain3D {
			a.settings.TranslationProfile = "3d"
		}
	}
	found := false
	for _, p := range profiles {
		if p.ID == a.settings.TranslationProfile {
			found = true
		}
	}
	if !found {
		a.profilesError = locales.ErrProfileRemoved()
	}
}

// profileName says built-in styles in the app's language. A user's own
// SKILL.md keeps the name its author wrote, even if it reuses a built-in ID.
func profileName(p translator.Profile) string {
	if p.Custom {
		return p.Name
	}
	switch p.ID {
	case "general":
		return locales.ProfileGeneral()
	case "3d":
		return locales.Profile3d()
	case "drama":
		return locales.ProfileDrama()
	case "literature":
		return locales.ProfileLiterature()
	case "news":
		return locales.ProfileNews()
	case "science":
		return locales.ProfileScience()
	case "short-video":
		return locales.ProfileShortVideo()
	case "teaching":
		return locales.ProfileTeaching()
	}
	return p.Name
}
