package main

import (
	"context"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
)

func (a *NativeApp) fetchProviderModels() {
	if a.modelsLoading || a.win == nil || a.settings.ProviderURL == "" {
		return
	}
	endpoint := a.settings.ProviderURL
	a.modelsEndpoint = endpoint
	key, err := security.ReadKey(endpoint)
	if err != nil {
		a.modelsError = err.Error()
		return
	}
	a.modelsLoading = true
	a.modelsError = ""
	go func() {
		names, err := translator.Models(context.Background(), endpoint, key, nil)
		a.win.Update(func() {
			a.modelsLoading = false
			if a.settings.ProviderURL != endpoint {
				return
			}
			if err != nil {
				a.modelsError = err.Error()
				return
			}
			a.providerModels = names
		})
	}()
}
