//go:build !windows

package jobs

import (
	"errors"
	"os"
	"os/exec"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type processGroup struct{}

func newProcessGroup() (*processGroup, error) {
	return nil, errors.New(locales.ErrJobPlatform())
}
func (g *processGroup) attach(*os.Process) error { return nil }
func (g *processGroup) terminate()               {}
func (g *processGroup) close()                   {}
func configure(*exec.Cmd)                        {}
