//go:build windows

package updates

import "os/exec"

// setSysProcAttr is a no-op on Windows. Process group isolation is not
// required because SIGHUP is a Unix-only concept.
func setSysProcAttr(cmd *exec.Cmd) {}
