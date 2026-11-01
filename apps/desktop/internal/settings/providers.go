package settings

import (
	"net/url"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
)

// Provider owns its model catalog and last selection. Secrets remain in the
// Windows credential store, scoped to this identity and the exact endpoint.
//
// Models are the ones the user keeps for this provider: several may be kept
// at once, and any of them may be the one in use. Catalog is what the
// endpoint offered when it was last asked: the list the user keeps models
// from. A model kept by hand, with its own limits, stays kept.
type Provider struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	URL       string                 `json:"url"`
	API       string                 `json:"api"`
	Model     string                 `json:"model"`
	Models    []translator.ModelInfo `json:"models,omitempty"`
	Catalog   []translator.ModelInfo `json:"catalog,omitempty"`
	LegacyKey bool                   `json:"legacyKey,omitempty"`
}

// KeepsModel reports whether the model is kept for this provider.
func (p *Provider) KeepsModel(id string) bool {
	for _, m := range p.Models {
		if m.ID == id {
			return true
		}
	}
	return false
}

// KeepModel keeps a model, with the limits the endpoint advertises for it,
// without touching the limits the user entered by hand.
func (p *Provider) KeepModel(info translator.ModelInfo) {
	for i, m := range p.Models {
		if m.ID != info.ID {
			continue
		}
		if m.LimitsSource != "manual" {
			p.Models[i] = info
		}
		return
	}
	p.Models = append(p.Models, info)
}

// DropModel takes a model out of the kept ones.
func (p *Provider) DropModel(id string) {
	for i, m := range p.Models {
		if m.ID == id {
			p.Models = append(p.Models[:i], p.Models[i+1:]...)
			return
		}
	}
}

func (s *Settings) Provider() *Provider {
	for i := range s.Providers {
		if s.Providers[i].ID == s.ActiveProvider {
			return &s.Providers[i]
		}
	}
	return nil
}

func (s *Settings) restoreProviders() {
	if len(s.Providers) == 0 && strings.TrimSpace(s.ProviderURL) != "" {
		name := s.ProviderURL
		if u, err := url.Parse(s.ProviderURL); err == nil && u.Host != "" {
			name = u.Host
		}
		s.Providers = []Provider{{ID: "legacy", Name: name, URL: s.ProviderURL, API: s.ProviderAPI, Model: s.ProviderModel, LegacyKey: true}}
		s.ActiveProvider = "legacy"
	}
	p := s.Provider()
	if p == nil && len(s.Providers) > 0 {
		s.ActiveProvider = s.Providers[0].ID
		p = &s.Providers[0]
	}
	if p != nil {
		s.ProviderURL, s.ProviderAPI, s.ProviderModel = p.URL, p.API, p.Model
		// Settings written before the offered models were kept apart from
		// the kept ones: the offered list becomes the catalog, and only the
		// model in use stays kept, with the limits it had.
		if len(p.Catalog) == 0 && len(p.Models) > 0 {
			p.Catalog = append([]translator.ModelInfo{}, p.Models...)
			kept := translator.ModelInfo{}
			for _, m := range p.Models {
				if m.ID == p.Model {
					kept = m
					break
				}
			}
			p.Models = nil
			if p.Model != "" {
				if kept.ID == "" {
					kept.ID = p.Model
				}
				p.Models = append(p.Models, kept)
			}
		}
	}
	s.SyncProvider()
}

// SyncProvider keeps the legacy active fields for existing installs and MCP
// clients; other saved providers are never overwritten by editing this one.
func (s *Settings) SyncProvider() {
	if p := s.Provider(); p != nil {
		p.URL, p.API, p.Model = s.ProviderURL, s.ProviderAPI, s.ProviderModel
		if p.Model != "" && s.ModelInfo() == nil {
			p.Models = append(p.Models, translator.ModelInfo{ID: p.Model})
		}
	}
}

func (s *Settings) SelectProvider(id string) bool {
	for i := range s.Providers {
		if s.Providers[i].ID != id {
			continue
		}
		s.SyncProvider()
		s.ActiveProvider = id
		p := &s.Providers[i]
		s.ProviderURL, s.ProviderAPI, s.ProviderModel = p.URL, p.API, p.Model
		return true
	}
	return false
}

func (s *Settings) ModelInfo() *translator.ModelInfo {
	if p := s.Provider(); p != nil {
		for i := range p.Models {
			if p.Models[i].ID == s.ProviderModel {
				return &p.Models[i]
			}
		}
	}
	return nil
}

// CredentialScope prevents a saved key following an edited endpoint. The one
// migrated provider keeps the existing endpoint-key association without copying
// a secret into settings or using another provider's key as a fallback.
func (s *Settings) CredentialScope() string {
	if p := s.Provider(); p != nil && !p.LegacyKey {
		return "provider:" + p.ID + ":" + strings.TrimRight(strings.TrimSpace(s.ProviderURL), "/")
	}
	return s.ProviderURL
}
