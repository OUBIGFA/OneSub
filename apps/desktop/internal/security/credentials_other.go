//go:build !windows

package security

import (
	"errors"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func SaveKey(endpoint, key string) error {
	return errors.New(locales.ErrCredentialsPlatform())
}
func ReadKey(endpoint string) (string, error) {
	return "", errors.New(locales.ErrCredentialsPlatform())
}
func DeleteKey(endpoint string) error {
	return errors.New(locales.ErrCredentialsPlatform())
}
