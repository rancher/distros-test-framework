//go:build windows

package qainfra

import "os/exec"

// killProcessGroupOnCancel is a no-op on Windows (CommandContext kills the parent only).
func killProcessGroupOnCancel(_ *exec.Cmd) {}
