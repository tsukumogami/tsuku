package main

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

func TestResolveMode_FlagWins(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "auto"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "suggest")

	m, err := resolveMode("confirm", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm", m)
	}
}

func TestResolveMode_EnvWinsOverConfig(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "confirm"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "suggest")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeSuggest {
		t.Errorf("got %v, want ModeSuggest", m)
	}
}

func TestResolveMode_ConfigWinsOverDefault(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "suggest"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeSuggest {
		t.Errorf("got %v, want ModeSuggest", m)
	}
}

func TestResolveMode_DefaultIsConfirm(t *testing.T) {
	cfg := &userconfig.Config{}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm", m)
	}
}

func TestResolveMode_EscalationRestriction_EnvAutoWithoutConfigAuto(t *testing.T) {
	// Env var says auto, but config does not corroborate. Must fall back to confirm.
	cfg := &userconfig.Config{AutoInstallMode: "confirm"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "auto")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm (escalation blocked)", m)
	}
}

func TestResolveMode_EscalationRestriction_EnvAutoWithEmptyConfig(t *testing.T) {
	// Env var says auto, config is empty (default). Must fall back to confirm.
	cfg := &userconfig.Config{}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "auto")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm (escalation blocked)", m)
	}
}

func TestResolveMode_EscalationAllowed_EnvAutoWithConfigAuto(t *testing.T) {
	// Both env and config say auto. Escalation is allowed.
	cfg := &userconfig.Config{AutoInstallMode: "auto"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "auto")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeAuto {
		t.Errorf("got %v, want ModeAuto", m)
	}
}

func TestResolveMode_EnvDowngrade_ConfigAutoEnvConfirm(t *testing.T) {
	// Config says auto, env says confirm. Downgrade is allowed.
	cfg := &userconfig.Config{AutoInstallMode: "auto"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "confirm")

	m, err := resolveMode("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m != autoinstall.ModeConfirm {
		t.Errorf("got %v, want ModeConfirm (downgrade)", m)
	}
}

func TestResolveMode_InvalidFlag(t *testing.T) {
	cfg := &userconfig.Config{}
	_, err := resolveMode("invalid", cfg)
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
}

func TestResolveMode_InvalidEnvVar(t *testing.T) {
	cfg := &userconfig.Config{}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "invalid")

	_, err := resolveMode("", cfg)
	if err == nil {
		t.Fatal("expected error for invalid env var")
	}
}

func TestResolveMode_InvalidConfig(t *testing.T) {
	cfg := &userconfig.Config{AutoInstallMode: "invalid"}
	t.Setenv("TSUKU_AUTO_INSTALL_MODE", "")

	_, err := resolveMode("", cfg)
	if err == nil {
		t.Fatal("expected error for invalid config value")
	}
}
