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
				a.errorText = err.Error()
				return
			}
			first := len(a.styleCatalog.Presets) == 0
			a.styleCatalog = catalog
			if first {
				a.currentStyle = catalog.DefaultStyle
				a.stylePreset = "默认"
				if a.settings.LastStyle != nil {
					a.currentStyle = *a.settings.LastStyle
					a.stylePreset = a.settings.StylePreset
				}
			}
			a.previewRevision++
			a.previewPending = true
		})
	}()
}
