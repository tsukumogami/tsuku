package main

import (
	"strings"
	"testing"
)

// TestHookEnvUnderARefusedConfig covers R7.
//
// The prompt hook is the one place where saying it twice is worse than not
// saying it: it fires on every prompt, so the line has to appear on arrival and
// then stop. And its stdout is what the shell evaluates, so nothing about the
// refusal may go there.
func TestHookEnvUnderARefusedConfig(t *testing.T) {
	work, _ := refusedLayout(t)
	chdir(t, work)

	// Each call feeds the previous call's exported tracking variables back in,
	// the way the shell hook does.
	carry := func(t *testing.T, stdout string) {
		t.Helper()
		for _, name := range []string{"_TSUKU_DIR", "_TSUKU_PREV_PATH", "_TSUKU_STATE_STAMP"} {
			if v, ok := exportedValue(stdout, name); ok {
				t.Setenv(name, v)
			}
		}
	}

	first := runCommandOutcome(t, func() error { return hookEnvCmd.RunE(hookEnvCmd, []string{"bash"}) })
	if first.exited && first.exit != 0 {
		t.Fatalf("hook-env must exit 0 under a refused config, got %d", first.exit)
	}
	if !strings.Contains(first.stderr, "not applied") {
		t.Fatalf("the first prompt did not report the refusal:\n%s", first.stderr)
	}
	if strings.Contains(first.stdout, "not applied") {
		t.Fatalf("the refusal reached the stream the shell evaluates:\n%s", first.stdout)
	}
	carry(t, first.stdout)

	second := runCommandOutcome(t, func() error { return hookEnvCmd.RunE(hookEnvCmd, []string{"bash"}) })
	if strings.Contains(second.stderr, "not applied") {
		t.Fatalf("the refusal repeated on the next prompt in the same directory:\n%s", second.stderr)
	}
}

// TestHookEnvRespectsQuietForARefusal pins the one place --quiet applies to a
// refusal: the prompt hook, which is the only caller that fires unbidden.
func TestHookEnvRespectsQuietForARefusal(t *testing.T) {
	work, _ := refusedLayout(t)
	chdir(t, work)

	quietFlag = true
	t.Cleanup(func() { quietFlag = false })

	outcome := runCommandOutcome(t, func() error { return hookEnvCmd.RunE(hookEnvCmd, []string{"bash"}) })
	if strings.Contains(outcome.stderr, "not applied") {
		t.Fatalf("--quiet did not suppress the refusal for hook-env:\n%s", outcome.stderr)
	}
}

// TestShellUnderARefusedConfig covers R8.
//
// tsuku shell is invoked deliberately, once, so it reports every time and
// --quiet does not apply. It must not fall through to the no-project branch,
// which prints a different message and exits non-zero.
func TestShellUnderARefusedConfig(t *testing.T) {
	work, _ := refusedLayout(t)
	chdir(t, work)

	for _, q := range []bool{false, true} {
		quietFlag = q
		first := runCommandOutcome(t, func() error { return shellCmd.RunE(shellCmd, nil) })
		if first.exited && first.exit != 0 {
			t.Fatalf("quiet=%v: tsuku shell must exit 0 under a refused config, got %d", q, first.exit)
		}
		if !strings.Contains(first.stderr, "not applied") {
			t.Fatalf("quiet=%v: no refusal reported:\n%s", q, first.stderr)
		}
		if strings.Contains(first.stderr, "no .tsuku.toml found") {
			t.Fatalf("quiet=%v: fell through to the no-project branch:\n%s", q, first.stderr)
		}
		if strings.Contains(first.stdout, "PATH=") {
			t.Fatalf("quiet=%v: stdout changed PATH under a refusal:\n%s", q, first.stdout)
		}

		second := runCommandOutcome(t, func() error { return shellCmd.RunE(shellCmd, nil) })
		if !strings.Contains(second.stderr, "not applied") {
			t.Fatalf("quiet=%v: the second invocation reported nothing:\n%s", q, second.stderr)
		}
	}
	quietFlag = false
}

// TestRunUnderARefusedConfigReportsAndCarriesOn covers R9: the refusal is
// printed and the command then behaves as though no config were present, rather
// than failing.
func TestRunUnderARefusedConfigReportsAndCarriesOn(t *testing.T) {
	work, _ := refusedLayout(t)

	outcome := runCommandOutcome(t, func() error {
		cfg, err := loadProjectConfigReporting(work)
		if cfg != nil {
			t.Error("a refused config was handed to the caller")
		}
		_ = err
		return nil
	})

	if !strings.Contains(outcome.stderr, "not applied") {
		t.Fatalf("no refusal reported:\n%s", outcome.stderr)
	}
}

// exportedValue pulls one variable out of the shell code hook-env emits.
func exportedValue(shellCode, name string) (string, bool) {
	for _, line := range strings.Split(shellCode, "\n") {
		prefix := "export " + name + "="
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		v := strings.TrimPrefix(line, prefix)
		v = strings.Trim(v, `"'`)
		return v, true
	}
	return "", false
}
