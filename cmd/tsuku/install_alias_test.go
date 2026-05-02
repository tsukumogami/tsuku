package main

import (
	"strings"
	"testing"
)

func TestResolveMultiSatisfier_PickFlag(t *testing.T) {
	candidates := []string{"corretto", "microsoft-openjdk", "openjdk", "temurin"}

	t.Run("valid pick", func(t *testing.T) {
		installPick = "temurin"
		t.Cleanup(func() { installPick = "" })

		got, err := resolveMultiSatisfier("java", candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "temurin" {
			t.Errorf("got %q, want temurin", got)
		}
	})

	t.Run("invalid pick", func(t *testing.T) {
		installPick = "nonexistent"
		t.Cleanup(func() { installPick = "" })

		_, err := resolveMultiSatisfier("java", candidates)
		if err == nil {
			t.Fatal("expected error for invalid pick")
		}
		if !strings.Contains(err.Error(), "nonexistent") || !strings.Contains(err.Error(), "java") {
			t.Errorf("error should name both the bad pick and the alias; got: %v", err)
		}
	})
}

func TestResolveMultiSatisfier_FromRecipeName(t *testing.T) {
	candidates := []string{"corretto", "openjdk", "temurin"}

	t.Run("recipe-name form (no colon)", func(t *testing.T) {
		installFrom = "openjdk"
		t.Cleanup(func() { installFrom = "" })

		got, err := resolveMultiSatisfier("java", candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "openjdk" {
			t.Errorf("got %q, want openjdk", got)
		}
	})

	t.Run("ecosystem-form (has colon) does not engage", func(t *testing.T) {
		installFrom = "homebrew:something"
		installYes = true
		t.Cleanup(func() {
			installFrom = ""
			installYes = false
		})

		// Colon present → not the alias-selection form. Falls through to
		// the non-interactive ambiguous error.
		_, err := resolveMultiSatisfier("java", candidates)
		if err == nil {
			t.Fatal("expected ambiguous error when --from has colon and -y is set")
		}
		if err != errAmbiguousAlias {
			t.Errorf("got error %v, want errAmbiguousAlias", err)
		}
	})

	t.Run("invalid recipe name", func(t *testing.T) {
		installFrom = "nonexistent"
		t.Cleanup(func() { installFrom = "" })

		_, err := resolveMultiSatisfier("java", candidates)
		if err == nil {
			t.Fatal("expected error for invalid --from recipe name")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error should name the bad recipe; got: %v", err)
		}
	})
}

func TestResolveMultiSatisfier_NonInteractiveErrors(t *testing.T) {
	candidates := []string{"openjdk", "temurin"}

	t.Run("-y returns ambiguous", func(t *testing.T) {
		installYes = true
		t.Cleanup(func() { installYes = false })

		_, err := resolveMultiSatisfier("java", candidates)
		if err != errAmbiguousAlias {
			t.Errorf("got %v, want errAmbiguousAlias", err)
		}
	})
}

func TestContainsColon(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"openjdk", false},
		{"homebrew:jq", true},
		{":leading", true},
		{"trailing:", true},
		{"a:b:c", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := containsColon(tt.in); got != tt.want {
				t.Errorf("containsColon(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
