package updates

import (
	"os"
	"strings"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// ShouldSuppressNotifications returns true when notification output should be
// silenced. The quiet parameter bridges the cmd/internal boundary -- callers in
// cmd/tsuku pass the --quiet flag value.
//
// Precedence (first match wins):
//  1. TSUKU_AUTO_UPDATE=1  -- explicit opt-in, never suppress
//  2. TSUKU_NO_UPDATE_CHECK=1 -- explicit opt-out, always suppress
//  3. CI=true -- environmental suppression
//  4. quiet flag -- user chose silence
//  5. Non-TTY stdout -- scripted context
//  6. Default -- don't suppress
func ShouldSuppressNotifications(quiet bool) bool {
	// Explicit opt-in overrides everything
	if os.Getenv(userconfig.EnvAutoUpdate) == "1" {
		return false
	}

	// Explicit opt-out
	if os.Getenv(userconfig.EnvNoUpdateCheck) == "1" {
		return true
	}

	// CI environment
	if strings.EqualFold(os.Getenv(userconfig.EnvCI), "true") {
		return true
	}

	// User chose quiet mode
	if quiet {
		return true
	}

	// Non-TTY stdout means scripted context
	if !progress.IsTerminalFunc(int(os.Stdout.Fd())) {
		return true
	}

	return false
}
