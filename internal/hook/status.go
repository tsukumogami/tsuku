package hook

import (
	"os"
	"path/filepath"
	"strings"
)

// Status reports whether the tsuku hook is installed for the given shell.
// homeDir is the user's home directory.
// It returns true if the hook is installed, false otherwise.
func Status(shell, homeDir string) (bool, error) {
	switch shell {
	case "bash", "zsh":
		return statusRCFile(shell, homeDir)
	case "fish":
		return statusFish(homeDir)
	default:
		return false, nil
	}
}

// statusRCFile checks whether the marker block is present in the rc file.
func statusRCFile(shell, homeDir string) (bool, error) {
	rcFile, err := rcFileForShell(shell, homeDir)
	if err != nil {
		return false, err
	}

	data, err := os.ReadFile(rcFile)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return strings.Contains(string(data), markerComment), nil
}

// statusFish checks whether ~/.config/fish/conf.d/tsuku.fish exists.
func statusFish(homeDir string) (bool, error) {
	dest := filepath.Join(homeDir, ".config", "fish", "conf.d", "tsuku.fish")
	_, err := os.Stat(dest)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
