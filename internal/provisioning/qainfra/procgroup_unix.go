//go:build !windows

package qainfra

import (
	"os/exec"
	"syscall"
	"time"
)

// killProcessGroupOnCancel starts cmd in its own process group and, when the
// context ends, kills that group (children included) instead of only the parent.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 10 * time.Second
}
