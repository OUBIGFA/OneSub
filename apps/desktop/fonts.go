package main

import (
	"context"
	"math"
)

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
				a.currentStyle = catalog.DefaultStyle
				a.stylePreset = "default"
				if a.settings.LastStyle != nil {
					a.currentStyle = *a.settings.LastStyle
					a.stylePreset = a.presetID(a.settings.StylePreset)
				}
				a.secondaryStyle = a.currentStyle
				// An unconfigured secondary layer inherits the chosen primary
				// typography at 80%; saved independent secondary styles stay intact.
				a.secondaryStyle.FontSize = math.Round(a.currentStyle.FontSize*8) / 10
				a.secondaryPreset = "default"
				if a.settings.SecondaryStyle != nil {
					a.secondaryStyle = *a.settings.SecondaryStyle
					a.secondaryPreset = a.presetID(a.settings.SecondaryPreset)
				}
			}
			a.previewRevision++
			a.previewPending = true
		})
	}()
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
