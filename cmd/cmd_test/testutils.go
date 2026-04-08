package cmd_test

import (
	"fmt"
	"os/exec"
)

type MockCmdExec struct {
	RunFunc    func(cmd *exec.Cmd) error
	OutputFunc func(cmd *exec.Cmd) ([]byte, error)
}

func (e MockCmdExec) Run(cmd *exec.Cmd) error {
	if e.RunFunc == nil {
		return fmt.Errorf("MockCmdExec.RunFunc not set")
	}
	return e.RunFunc(cmd)
}

func (e MockCmdExec) Output(cmd *exec.Cmd) ([]byte, error) {
	if e.OutputFunc == nil {
		return nil, fmt.Errorf("MockCmdExec.OutputFunc not set")
	}
	return e.OutputFunc(cmd)
}
