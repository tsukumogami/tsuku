package updates

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/progress"
)

func TestShouldSuppressNotifications_Default(t *testing.T) {
	clearSuppressEnv(t)
	mockTTY(t, true)

	if ShouldSuppressNotifications(false) {
		t.Error("expected no suppression with defaults")
	}
}

func TestShouldSuppressNotifications_AutoUpdateOverride(t *testing.T) {
	// TSUKU_AUTO_UPDATE=1 should override all other suppression signals
	t.Setenv("TSUKU_AUTO_UPDATE", "1")
	t.Setenv("CI", "true")
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "")
	mockTTY(t, false)

	if ShouldSuppressNotifications(true) {
		t.Error("TSUKU_AUTO_UPDATE=1 should override all suppression, including quiet and CI")
	}
}

func TestShouldSuppressNotifications_NoUpdateCheck(t *testing.T) {
	clearSuppressEnv(t)
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "1")
	mockTTY(t, true)

	if !ShouldSuppressNotifications(false) {
		t.Error("TSUKU_NO_UPDATE_CHECK=1 should suppress")
	}
}

func TestShouldSuppressNotifications_CI(t *testing.T) {
	clearSuppressEnv(t)
	t.Setenv("CI", "true")
	mockTTY(t, true)

	if !ShouldSuppressNotifications(false) {
		t.Error("CI=true should suppress")
	}
}

func TestShouldSuppressNotifications_CI_CaseInsensitive(t *testing.T) {
	clearSuppressEnv(t)
	t.Setenv("CI", "True")
	mockTTY(t, true)

	if !ShouldSuppressNotifications(false) {
		t.Error("CI=True (mixed case) should suppress")
	}
}

func TestShouldSuppressNotifications_Quiet(t *testing.T) {
	clearSuppressEnv(t)
	mockTTY(t, true)

	if !ShouldSuppressNotifications(true) {
		t.Error("quiet=true should suppress")
	}
}

func TestShouldSuppressNotifications_NonTTY(t *testing.T) {
	clearSuppressEnv(t)
	mockTTY(t, false)

	if !ShouldSuppressNotifications(false) {
		t.Error("non-TTY stdout should suppress")
	}
}

func TestShouldSuppressNotifications_Precedence_AutoUpdateBeatsNoCheck(t *testing.T) {
	t.Setenv("TSUKU_AUTO_UPDATE", "1")
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "1")
	t.Setenv("CI", "")
	mockTTY(t, true)

	if ShouldSuppressNotifications(false) {
		t.Error("TSUKU_AUTO_UPDATE should take precedence over TSUKU_NO_UPDATE_CHECK")
	}
}

func TestShouldSuppressNotifications_Precedence_NoCheckBeatsCI(t *testing.T) {
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "1")
	t.Setenv("CI", "true")
	mockTTY(t, true)

	// Both suppress, but NO_UPDATE_CHECK should match first
	if !ShouldSuppressNotifications(false) {
		t.Error("expected suppression")
	}
}

func TestShouldSuppressNotifications_Precedence_CIBeatsQuiet(t *testing.T) {
	clearSuppressEnv(t)
	t.Setenv("CI", "true")
	mockTTY(t, true)

	// CI suppresses even when quiet is false
	if !ShouldSuppressNotifications(false) {
		t.Error("CI should suppress independently of quiet flag")
	}
}

// clearSuppressEnv clears all env vars that affect suppression.
func clearSuppressEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "")
	t.Setenv("CI", "")
}

// mockTTY overrides IsTerminalFunc for the duration of the test.
func mockTTY(t *testing.T, isTTY bool) {
	t.Helper()
	orig := progress.IsTerminalFunc
	progress.IsTerminalFunc = func(fd int) bool { return isTTY }
	t.Cleanup(func() { progress.IsTerminalFunc = orig })
}
