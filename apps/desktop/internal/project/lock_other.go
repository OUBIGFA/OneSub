//go:build !windows

package project

import (
	"errors"
	"os"
)

// Other platforms need their own lock and persistence acceptance before use.
func acquireLock(path string) (*os.File, error) {
	return nil, errors.New("此构建只验收 Windows 本地工程")
}
