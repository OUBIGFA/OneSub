// Package security keeps provider secrets in the Windows credential facility.
package security

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func credentialTarget(endpoint string) string {
	return fmt.Sprintf("OneSub/provider/%x", sha256.Sum256([]byte(strings.TrimRight(strings.TrimSpace(endpoint), "/"))))
}
