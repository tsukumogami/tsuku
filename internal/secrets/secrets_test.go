package secrets

import (
	"strings"
	"testing"
)

func TestGetResolvesFromEnvVar(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-123")

	val, err := Get("anthropic_api_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "sk-ant-test-123" {
		t.Errorf("expected 'sk-ant-test-123', got %q", val)
	}
}

func TestGetResolvesMultiAliasInPriorityOrder(t *testing.T) {
	// Both set: first alias wins.
	t.Setenv("GOOGLE_API_KEY", "google-key")
	t.Setenv("GEMINI_API_KEY", "gemini-key")

	val, err := Get("google_api_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "google-key" {
		t.Errorf("expected 'google-key' (first alias), got %q", val)
	}
}

func TestGetResolvesSecondAlias(t *testing.T) {
	// Only second alias set.
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "gemini-fallback")

	val, err := Get("google_api_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "gemini-fallback" {
		t.Errorf("expected 'gemini-fallback', got %q", val)
	}
}

func TestGetRejectsUnknownKey(t *testing.T) {
	_, err := Get("nonexistent_key")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown secret key") {
		t.Errorf("expected 'unknown secret key' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent_key") {
		t.Errorf("expected key name in error, got: %v", err)
	}
}

func TestGetReturnsGuidanceWhenNotSet(t *testing.T) {
	// Ensure the env var is unset.
	t.Setenv("ANTHROPIC_API_KEY", "")

	_, err := Get("anthropic_api_key")
	if err == nil {
		t.Fatal("expected error when secret is not set")
	}

	msg := err.Error()

	// Error should mention the env var name.
	if !strings.Contains(msg, "ANTHROPIC_API_KEY") {
		t.Errorf("expected env var name in error, got: %s", msg)
	}

	// Error should mention config file alternative.
	if !strings.Contains(msg, "config.toml") {
		t.Errorf("expected config.toml mention in error, got: %s", msg)
	}

	// Error should mention the key name.
	if !strings.Contains(msg, "anthropic_api_key") {
		t.Errorf("expected key name in error, got: %s", msg)
	}
}

func TestGetGuidanceListsAllAliases(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	_, err := Get("google_api_key")
	if err == nil {
		t.Fatal("expected error when secret is not set")
	}

	msg := err.Error()
	if !strings.Contains(msg, "GOOGLE_API_KEY") {
		t.Errorf("expected GOOGLE_API_KEY in error, got: %s", msg)
	}
	if !strings.Contains(msg, "GEMINI_API_KEY") {
		t.Errorf("expected GEMINI_API_KEY in error, got: %s", msg)
	}
}

func TestIsSetReturnsTrueWhenEnvSet(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	if !IsSet("github_token") {
		t.Error("expected IsSet to return true when env var is set")
	}
}

func TestIsSetReturnsFalseWhenEnvEmpty(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "")

	if IsSet("tavily_api_key") {
		t.Error("expected IsSet to return false when env var is empty")
	}
}

func TestIsSetReturnsFalseForUnknownKey(t *testing.T) {
	if IsSet("nonexistent_key") {
		t.Error("expected IsSet to return false for unknown key")
	}
}

func TestIsSetWithMultiAlias(t *testing.T) {
	// Only second alias set.
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "gemini-key")

	if !IsSet("google_api_key") {
		t.Error("expected IsSet to return true when any alias is set")
	}
}

func TestKnownKeysReturnsAllSecrets(t *testing.T) {
	keys := KnownKeys()

	if len(keys) != 5 {
		t.Fatalf("expected 5 known keys, got %d", len(keys))
	}

	// Verify sorted order.
	for i := 1; i < len(keys); i++ {
		if keys[i].Name < keys[i-1].Name {
			t.Errorf("keys not sorted: %q before %q", keys[i-1].Name, keys[i].Name)
		}
	}
}

func TestKnownKeysContainsExpectedEntries(t *testing.T) {
	keys := KnownKeys()

	expected := map[string]bool{
		"anthropic_api_key": false,
		"google_api_key":    false,
		"github_token":      false,
		"tavily_api_key":    false,
		"brave_api_key":     false,
	}

	for _, k := range keys {
		if _, ok := expected[k.Name]; !ok {
			t.Errorf("unexpected key: %q", k.Name)
		}
		expected[k.Name] = true
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing expected key: %q", name)
		}
	}
}

func TestKnownKeysFieldsPopulated(t *testing.T) {
	keys := KnownKeys()

	for _, k := range keys {
		if k.Name == "" {
			t.Error("KeyInfo.Name should not be empty")
		}
		if len(k.EnvVars) == 0 {
			t.Errorf("KeyInfo.EnvVars should not be empty for %q", k.Name)
		}
		if k.Desc == "" {
			t.Errorf("KeyInfo.Desc should not be empty for %q", k.Name)
		}
	}
}

func TestGoogleKeyHasMultipleEnvVars(t *testing.T) {
	keys := KnownKeys()

	for _, k := range keys {
		if k.Name == "google_api_key" {
			if len(k.EnvVars) != 2 {
				t.Fatalf("expected 2 env vars for google_api_key, got %d", len(k.EnvVars))
			}
			if k.EnvVars[0] != "GOOGLE_API_KEY" {
				t.Errorf("expected first env var to be GOOGLE_API_KEY, got %q", k.EnvVars[0])
			}
			if k.EnvVars[1] != "GEMINI_API_KEY" {
				t.Errorf("expected second env var to be GEMINI_API_KEY, got %q", k.EnvVars[1])
			}
			return
		}
	}
	t.Error("google_api_key not found in KnownKeys")
}

func TestGetAllKnownKeysFromEnv(t *testing.T) {
	// Verify each known key can be resolved from its primary env var.
	envValues := map[string]string{
		"ANTHROPIC_API_KEY": "anthropic-val",
		"GOOGLE_API_KEY":    "google-val",
		"GITHUB_TOKEN":      "github-val",
		"TAVILY_API_KEY":    "tavily-val",
		"BRAVE_API_KEY":     "brave-val",
	}
	for env, val := range envValues {
		t.Setenv(env, val)
	}

	keys := KnownKeys()
	for _, k := range keys {
		val, err := Get(k.Name)
		if err != nil {
			t.Errorf("Get(%q) returned error: %v", k.Name, err)
			continue
		}
		if val == "" {
			t.Errorf("Get(%q) returned empty value", k.Name)
		}
	}
}

func TestGetReturnsErrorWhenEnvVarEmpty(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "")

	_, err := Get("brave_api_key")
	if err == nil {
		t.Error("expected error when BRAVE_API_KEY is empty")
	}
}
