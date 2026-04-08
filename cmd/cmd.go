package cmd

import (
	"fmt"
	"os/exec"
	"strings"
)

type Executor interface {
	Run(cmd *exec.Cmd) error
	Output(cmd *exec.Cmd) ([]byte, error)
}

type Exec struct{}

func (e Exec) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

func (e Exec) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func MakeExecutor() Executor {
	return Exec{}
}

func ToString(cmd *exec.Cmd) string {
	if cmd == nil {
		return "<nil>"
	}
	return strings.Join(cmd.Args, " ")
}

type ExitCoder interface {
	ExitCode() int
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ExitError) ExitCode() int {
	if e == nil || e.Code == 0 {
		return 1
	}
	return e.Code
}

func NewExitError(code int, format string, args ...any) *ExitError {
	return &ExitError{
		Code: code,
		Err:  fmt.Errorf(format, args...),
	}
}
