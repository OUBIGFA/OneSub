package main

import (
	"math"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Exercise the actual credential store with an isolated provider, including
// repeated reveals and unsaved input, without printing credential contents.
func TestProviderKeyCanBeRevealedRepeatedly(t *testing.T) {
	inLocale(t, "en", func() {
		a := nativeForTest(t)
		a.addProvider()
		a.settings.ProviderURL = "https://example.invalid/v1"
		scope := a.settings.CredentialScope()
		if err := security.SaveKey(scope, "test-only-key"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { security.DeleteKey(scope) })
		tt := settled(a.providerConnectionFields, 650, 650)
		inputBounds, _ := tt.Find("API Key")
		for range 3 {
			button, ok := tt.Find("Show key")
			if !ok || button.X < inputBounds.X || button.X+button.W > inputBounds.X+inputBounds.W || button.Y < inputBounds.Y || button.Y+button.H > inputBounds.Y+inputBounds.H {
				t.Fatal("visibility icon is outside the input", button, inputBounds)
			}
			if math.Abs(float64(button.W-button.H)) > 0.01 || button.W > 32 {
				t.Fatal("visibility action is not an icon-sized button", button)
			}
			if err := tt.Click("Show key"); err != nil {
				t.Fatal(err)
			}
			if a.providerKey != "test-only-key" {
				t.Fatal("saved credential was not loaded")
			}
			if err := tt.Click("Hide key"); err != nil {
				t.Fatal(err)
			}
			if bounds, _ := tt.Find("API Key"); bounds != inputBounds {
				t.Fatal("visibility toggle moved the input", bounds, inputBounds)
			}
		}
		a.providerKey = "unsaved-test-key"
		tt.Frame()
		if err := tt.Click("Show key"); err != nil {
			t.Fatal(err)
		}
		if a.providerKey != "unsaved-test-key" {
			t.Fatal("revealing replaced the draft")
		}
		if err := tt.Click(locales.TranslationSaveKey()); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if err := tt.Click("Show key"); err != nil {
			t.Fatal(err)
		}
		if a.providerKey != "unsaved-test-key" {
			t.Fatal("saved credential cannot be revealed again")
		}
		a.addProvider()
		tt.Frame()
		if a.providerKey != "" {
			t.Fatal("credential crossed provider boundary")
		}
		if _, ok := tt.Find("Show key"); !ok {
			t.Fatal("new provider inherited visible credentials")
		}
	})
}
