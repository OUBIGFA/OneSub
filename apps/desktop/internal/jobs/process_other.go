//go:build !windows

package jobs

import (
	"errors"
	"os"
	"os/exec"
)

type processGroup struct{}

func newProcessGroup() (*processGroup, error) {
	return nil, errors.New("此构建尚未验收当前平台的任务取消")
}
func (g *processGroup) attach(*os.Process) error { return nil }
func (g *processGroup) terminate()               {}
func (g *processGroup) close()                   {}
func configure(*exec.Cmd)                        {}
