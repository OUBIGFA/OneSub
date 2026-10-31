package main

import "context"

// Refresh only catalog data after startup; never reset a user's chosen style
// because an installed font changed while they were editing subtitles.
func (a *NativeApp) reloadFonts() {
	if a.fontCatalogLoading || a.win == nil {
		return
	}
	a.fontCatalogLoading = true
	go func() {
		catalog, err := a.styleClient.Catalog(context.Background())
		a.win.Update(func() {
			a.fontCatalogLoading = false
			if err != nil {
				a.showError("fonts", err.Error())
				return
			}
			a.clearError("fonts")
			first := len(a.styleCatalog.Presets) == 0
			a.styleCatalog = catalog
			if first {
				a.restoreStyles()
			}
			a.previewRevision++
			a.previewPending = true
		})
	}()
}

// restoreStyles initializes both independent layers once. Half-size is only
// the starting point: reloading fonts or changing language priority must never
// replace a user's saved secondary typography, even when it equals an old default.
func (a *NativeApp) restoreStyles() {
	a.currentStyle, a.stylePreset = a.styleCatalog.DefaultStyle, "default"
	if a.settings.LastStyle != nil {
		a.currentStyle, a.stylePreset = *a.settings.LastStyle, a.presetID(a.settings.StylePreset)
	}
	a.secondaryStyle, a.secondaryPreset = a.currentStyle, "default"
	a.secondaryStyle.FontSize = a.currentStyle.FontSize / 2
	if a.settings.SecondaryStyle != nil {
		a.secondaryStyle, a.secondaryPreset = *a.settings.SecondaryStyle, a.presetID(a.settings.SecondaryPreset)
	}
}

// presetID reads a saved preset choice. Settings before 0.0.1 build
// 20261028 stored the Chinese display name instead of the ID.
func (a *NativeApp) presetID(saved string) string {
	if saved == "" || saved == "默认" {
		return "default"
	}
	for _, p := range a.styleCatalog.Presets {
		if p.ID == saved || p.Name == saved {
			return p.ID
		}
	}
	return saved
}
