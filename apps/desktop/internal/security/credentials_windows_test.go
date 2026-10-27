package security

import (
	"github.com/google/uuid"
	"testing"
)

func TestProviderKeysAreScopedAndRoundTrip(t *testing.T) {
	endpoint := "https://test.invalid/onesub/" + uuid.NewString()
	defer DeleteKey(endpoint)
	if err := SaveKey(endpoint, "test-only-not-a-live-key"); err != nil {
		t.Fatal(err)
	}
	key, err := ReadKey(endpoint)
	if err != nil || key != "test-only-not-a-live-key" {
		t.Fatal("credential round trip failed", err)
	}
	other, err := ReadKey(endpoint + "/other")
	if err != nil || other != "" {
		t.Fatal("provider scope failed", err)
	}
	if err = DeleteKey(endpoint); err != nil {
		t.Fatal(err)
	}
	key, err = ReadKey(endpoint)
	if err != nil || key != "" {
		t.Fatal("credential delete failed", err)
	}
}
