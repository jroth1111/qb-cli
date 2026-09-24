//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

func detachKeeper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
}
