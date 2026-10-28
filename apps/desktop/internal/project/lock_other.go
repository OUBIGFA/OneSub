//go:build !windows

package project

import (
	"errors"
	"os"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Other platforms need their own lock and persistence acceptance before use.
func acquireLock(path string) (*os.File, error) {
	return nil, errors.New(locales.ErrProjectPlatform())
}
