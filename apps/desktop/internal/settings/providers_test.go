package settings

import (
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
)

// Saving must not resurrect a removed active model through the legacy fields.
func TestProviderCatalogRefreshRemovesUnavailableSelections(t *testing.T) {
	s := Default()
	s.Providers = []Provider{
		{ID: "active", URL: "https://example.com/v1", Model: "expired", Models: []translator.ModelInfo{{ID: "expired"}, {ID: "keep"}, {ID: "manual-expired", LimitsSource: "manual"}}},
		{ID: "other", Model: "other-model", Models: []translator.ModelInfo{{ID: "other-model"}}},
	}
	s.ActiveProvider = "active"
	s.ProviderURL, s.ProviderModel = s.Providers[0].URL, "expired"
	s.ReplaceProviderCatalog([]translator.ModelInfo{{ID: "keep", ContextWindow: 32000}, {ID: "new"}})
	if s.ProviderModel != "" || s.Provider().Model != "" || len(s.Provider().Models) != 1 || !s.Provider().KeepsModel("keep") || s.Provider().KeepsModel("new") {
		t.Fatal("refresh retained a missing model or selected an unchecked one", s.Provider())
	}
	if s.Provider().Models[0].ContextWindow != 32000 || s.Providers[1].Model != "other-model" || len(s.Providers[1].Models) != 1 {
		t.Fatal("refresh lost live metadata or changed another provider", s.Providers)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.ProviderModel != "" || len(loaded.Provider().Models) != 1 || len(loaded.Provider().Catalog) != 2 {
		t.Fatal("restart resurrected an unavailable model", err, loaded.Provider())
	}
}

func TestProviderCatalogRefreshPreservesLiveSelectionAndManualLimits(t *testing.T) {
	s := Default()
	s.Providers = []Provider{{ID: "active", Model: "keep", Models: []translator.ModelInfo{
		{ID: "keep", MaxOutputTokens: 1234, LimitsSource: "manual"},
		{ID: "advertised", MaxInputTokens: 1000},
	}}}
	s.ActiveProvider, s.ProviderModel = "active", "keep"
	s.ReplaceProviderCatalog([]translator.ModelInfo{
		{ID: "keep", MaxOutputTokens: 8192},
		{ID: "advertised", MaxInputTokens: 96000},
		{ID: "new"},
	})
	if s.ProviderModel != "keep" || s.Provider().Model != "keep" || len(s.Provider().Models) != 2 || s.ModelInfo().MaxOutputTokens != 1234 || s.ModelInfo().LimitsSource != "manual" {
		t.Fatal("refresh changed the live selection or manual limits", s.Provider())
	}
	if s.Provider().Models[1].MaxInputTokens != 96000 || s.Provider().KeepsModel("new") {
		t.Fatal("refresh failed to update advertised limits or enabled a new model", s.Provider())
	}
}

func TestProviderCatalogRefreshRemovesTheLastUnavailableModel(t *testing.T) {
	s := Default()
	s.Providers = []Provider{{ID: "active", Model: "expired", Models: []translator.ModelInfo{{ID: "expired"}}, Catalog: []translator.ModelInfo{{ID: "expired"}}}}
	s.ActiveProvider, s.ProviderModel = "active", "expired"
	s.ReplaceProviderCatalog([]translator.ModelInfo{})
	s.SyncProvider()
	if s.ProviderModel != "" || s.Provider().Model != "" || len(s.Provider().Models) != 0 || len(s.Provider().Catalog) != 0 {
		t.Fatal("empty refreshed catalog retained the last model", s.Provider())
	}
}
