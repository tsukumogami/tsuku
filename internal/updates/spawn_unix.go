//go:build !windows

package updates

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr places cmd in its own process group on Unix so that
// SIGHUP is not propagated to the background subprocess when the
// parent terminal closes.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
