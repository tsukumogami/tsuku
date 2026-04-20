//go:build !windows

package updates

import (
	"os/exec"
	"testing"
)

func TestSetSysProcAttr(t *testing.T) {
	cmd := exec.Command("true")
	setSysProcAttr(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be non-nil after setSysProcAttr")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Fatal("expected SysProcAttr.Setpgid to be true after setSysProcAttr")
	}
}

func TestSpawnDetachedNonexistentBinary(t *testing.T) {
	cmd := exec.Command("/nonexistent/binary/path/that/does/not/exist")
	err := spawnDetached(cmd)
	if err == nil {
		t.Fatal("expected non-nil error from spawnDetached with nonexistent binary, got nil")
	}
}
