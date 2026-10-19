//go:build windows

package iproute

import (
	"os/exec"
	"syscall"
)

// hideWindow sets the HideWindow flag for Windows commands to prevent popping up console windows.
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
