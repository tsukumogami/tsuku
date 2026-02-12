// Package addon provides stub functions for the tsuku-llm addon binary.
// This package manages the path and existence checks for the addon.
// Full download logic is implemented in a downstream issue.
package addon

import (
	"os"
	"path/filepath"
	"runtime"
)

// AddonPath returns the path to the tsuku-llm binary.
func AddonPath() string {
	home := os.Getenv("TSUKU_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(userHome, ".tsuku")
	}

	binName := "tsuku-llm"
	if runtime.GOOS == "windows" {
		binName = "tsuku-llm.exe"
	}

	return filepath.Join(home, "tools", "tsuku-llm", binName)
}

// IsInstalled checks if the addon is installed.
func IsInstalled() bool {
	_, err := os.Stat(AddonPath())
	return err == nil
}
