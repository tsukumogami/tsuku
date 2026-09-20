package main

import (
	"os"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
)

// A verify command is the one place a recipe's documented $TSUKU_HOME
// convention is evaluated by a shell. Without it in the environment the
// variable expands to empty and `test -d $TSUKU_HOME/apps/x.app` quietly
// tests `/apps/x.app` (tsukumogami/tsuku#2609).
func TestMakeVerifyEnvSetsTsukuHome(t *testing.T) {
	cfg := &config.Config{HomeDir: "/tmp/tsuku-home", CurrentDir: "/tmp/tsuku-home/tools/current"}

	env := makeVerifyEnv("/tmp/tsuku-home/tools/x-1.0", cfg)

	var got string
	found := 0
	for _, e := range env {
		if strings.HasPrefix(e, "TSUKU_HOME=") {
			got = strings.TrimPrefix(e, "TSUKU_HOME=")
			found++
		}
	}
	if found == 0 {
		t.Fatal("verify environment does not set TSUKU_HOME")
	}
	if found != 1 {
		t.Fatalf("TSUKU_HOME set %d times; a duplicate makes the effective value depend on who reads the slice", found)
	}
	if got != cfg.HomeDir {
		t.Fatalf("TSUKU_HOME = %q, want %q", got, cfg.HomeDir)
	}
}

// The configured home wins over whatever the ambient shell exported, so the
// verify environment agrees with the home this run actually used.
func TestMakeVerifyEnvOverridesAmbientTsukuHome(t *testing.T) {
	t.Setenv("TSUKU_HOME", "/somewhere/else")
	cfg := &config.Config{HomeDir: "/tmp/tsuku-home", CurrentDir: "/tmp/tsuku-home/tools/current"}

	for _, e := range makeVerifyEnv("/tmp/tsuku-home/tools/x-1.0", cfg) {
		if e == "TSUKU_HOME=/somewhere/else" {
			t.Fatal("ambient TSUKU_HOME leaked into the verify environment")
		}
	}
	_ = os.Environ
}
