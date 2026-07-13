//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
