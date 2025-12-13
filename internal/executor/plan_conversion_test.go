package executor

import (
	"reflect"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/install"
)

func TestToStoragePlan(t *testing.T) {
	t.Run("nil plan returns nil", func(t *testing.T) {
		result := ToStoragePlan(nil)
		if result != nil {
			t.Errorf("ToStoragePlan(nil) = %v, want nil", result)
		}
	})

	t.Run("converts complete plan", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		plan := &InstallationPlan{
			FormatVersion: 1,
			Tool:          "gh",
			Version:       "2.40.0",
			Platform: Platform{
				OS:   "linux",
				Arch: "amd64",
			},
			GeneratedAt:   now,
			RecipeHash:    "abc123",
			RecipeSource:  "registry",
			Deterministic: true,
			Steps: []ResolvedStep{
				{
					Action:        "download_archive",
					Params:        map[string]interface{}{"url": "https://example.com/file.tar.gz"},
					Evaluable:     true,
					Deterministic: true,
					URL:           "https://example.com/file.tar.gz",
					Checksum:      "sha256:deadbeef",
					Size:          12345,
				},
				{
					Action:        "extract",
					Params:        map[string]interface{}{"format": "tar.gz"},
					Evaluable:     true,
					Deterministic: true,
				},
			},
		}

		result := ToStoragePlan(plan)
		if result == nil {
			t.Fatal("ToStoragePlan returned nil for non-nil input")
		}

		// Verify top-level fields
		if result.FormatVersion != 1 {
			t.Errorf("FormatVersion = %d, want 1", result.FormatVersion)
		}
		if result.Tool != "gh" {
			t.Errorf("Tool = %q, want %q", result.Tool, "gh")
		}
		if result.Version != "2.40.0" {
			t.Errorf("Version = %q, want %q", result.Version, "2.40.0")
		}
		if result.Platform.OS != "linux" || result.Platform.Arch != "amd64" {
			t.Errorf("Platform = %+v, want linux/amd64", result.Platform)
		}
		if !result.GeneratedAt.Equal(now) {
			t.Errorf("GeneratedAt = %v, want %v", result.GeneratedAt, now)
		}
		if result.RecipeHash != "abc123" {
			t.Errorf("RecipeHash = %q, want %q", result.RecipeHash, "abc123")
		}
		if result.RecipeSource != "registry" {
			t.Errorf("RecipeSource = %q, want %q", result.RecipeSource, "registry")
		}
		if !result.Deterministic {
			t.Error("Deterministic = false, want true")
		}

		// Verify steps
		if len(result.Steps) != 2 {
			t.Fatalf("len(Steps) = %d, want 2", len(result.Steps))
		}

		step0 := result.Steps[0]
		if step0.Action != "download_archive" {
			t.Errorf("Steps[0].Action = %q, want %q", step0.Action, "download_archive")
		}
		if !step0.Evaluable {
			t.Error("Steps[0].Evaluable = false, want true")
		}
		if !step0.Deterministic {
			t.Error("Steps[0].Deterministic = false, want true")
		}
		if step0.URL != "https://example.com/file.tar.gz" {
			t.Errorf("Steps[0].URL = %q, want %q", step0.URL, "https://example.com/file.tar.gz")
		}
		if step0.Checksum != "sha256:deadbeef" {
			t.Errorf("Steps[0].Checksum = %q, want %q", step0.Checksum, "sha256:deadbeef")
		}
		if step0.Size != 12345 {
			t.Errorf("Steps[0].Size = %d, want 12345", step0.Size)
		}
	})
}

func TestFromStoragePlan(t *testing.T) {
	t.Run("nil plan returns nil", func(t *testing.T) {
		result := FromStoragePlan(nil)
		if result != nil {
			t.Errorf("FromStoragePlan(nil) = %v, want nil", result)
		}
	})

	t.Run("converts complete plan", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		plan := &install.Plan{
			FormatVersion: 1,
			Tool:          "gh",
			Version:       "2.40.0",
			Platform: install.PlanPlatform{
				OS:   "darwin",
				Arch: "arm64",
			},
			GeneratedAt:   now,
			RecipeHash:    "def456",
			RecipeSource:  "/path/to/recipe.toml",
			Deterministic: false,
			Steps: []install.PlanStep{
				{
					Action:        "go_build",
					Params:        map[string]interface{}{"module": "github.com/example/tool"},
					Evaluable:     false,
					Deterministic: false,
				},
			},
		}

		result := FromStoragePlan(plan)
		if result == nil {
			t.Fatal("FromStoragePlan returned nil for non-nil input")
		}

		// Verify top-level fields
		if result.FormatVersion != 1 {
			t.Errorf("FormatVersion = %d, want 1", result.FormatVersion)
		}
		if result.Tool != "gh" {
			t.Errorf("Tool = %q, want %q", result.Tool, "gh")
		}
		if result.Version != "2.40.0" {
			t.Errorf("Version = %q, want %q", result.Version, "2.40.0")
		}
		if result.Platform.OS != "darwin" || result.Platform.Arch != "arm64" {
			t.Errorf("Platform = %+v, want darwin/arm64", result.Platform)
		}
		if !result.GeneratedAt.Equal(now) {
			t.Errorf("GeneratedAt = %v, want %v", result.GeneratedAt, now)
		}
		if result.RecipeHash != "def456" {
			t.Errorf("RecipeHash = %q, want %q", result.RecipeHash, "def456")
		}
		if result.RecipeSource != "/path/to/recipe.toml" {
			t.Errorf("RecipeSource = %q, want %q", result.RecipeSource, "/path/to/recipe.toml")
		}
		if result.Deterministic {
			t.Error("Deterministic = true, want false")
		}

		// Verify steps
		if len(result.Steps) != 1 {
			t.Fatalf("len(Steps) = %d, want 1", len(result.Steps))
		}

		step0 := result.Steps[0]
		if step0.Action != "go_build" {
			t.Errorf("Steps[0].Action = %q, want %q", step0.Action, "go_build")
		}
		if step0.Evaluable {
			t.Error("Steps[0].Evaluable = true, want false")
		}
		if step0.Deterministic {
			t.Error("Steps[0].Deterministic = true, want false")
		}
	})
}

func TestRoundTripConversion(t *testing.T) {
	t.Run("executor to storage to executor preserves data", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		original := &InstallationPlan{
			FormatVersion: 1,
			Tool:          "kubectl",
			Version:       "1.29.0",
			Platform: Platform{
				OS:   "linux",
				Arch: "arm64",
			},
			GeneratedAt:   now,
			RecipeHash:    "hash123",
			RecipeSource:  "registry",
			Deterministic: true,
			Steps: []ResolvedStep{
				{
					Action:        "download_archive",
					Params:        map[string]interface{}{"url": "https://example.com/kubectl.tar.gz", "strip_prefix": float64(1)},
					Evaluable:     true,
					Deterministic: true,
					URL:           "https://example.com/kubectl.tar.gz",
					Checksum:      "sha256:abcd1234",
					Size:          50000000,
				},
				{
					Action:        "install_binaries",
					Params:        map[string]interface{}{"binaries": []interface{}{"kubectl"}},
					Evaluable:     true,
					Deterministic: true,
				},
			},
		}

		// Convert to storage format and back
		storage := ToStoragePlan(original)
		roundTripped := FromStoragePlan(storage)

		// Compare top-level fields
		if roundTripped.FormatVersion != original.FormatVersion {
			t.Errorf("FormatVersion: got %d, want %d", roundTripped.FormatVersion, original.FormatVersion)
		}
		if roundTripped.Tool != original.Tool {
			t.Errorf("Tool: got %q, want %q", roundTripped.Tool, original.Tool)
		}
		if roundTripped.Version != original.Version {
			t.Errorf("Version: got %q, want %q", roundTripped.Version, original.Version)
		}
		if roundTripped.Platform != original.Platform {
			t.Errorf("Platform: got %+v, want %+v", roundTripped.Platform, original.Platform)
		}
		if !roundTripped.GeneratedAt.Equal(original.GeneratedAt) {
			t.Errorf("GeneratedAt: got %v, want %v", roundTripped.GeneratedAt, original.GeneratedAt)
		}
		if roundTripped.RecipeHash != original.RecipeHash {
			t.Errorf("RecipeHash: got %q, want %q", roundTripped.RecipeHash, original.RecipeHash)
		}
		if roundTripped.RecipeSource != original.RecipeSource {
			t.Errorf("RecipeSource: got %q, want %q", roundTripped.RecipeSource, original.RecipeSource)
		}
		if roundTripped.Deterministic != original.Deterministic {
			t.Errorf("Deterministic: got %v, want %v", roundTripped.Deterministic, original.Deterministic)
		}

		// Compare steps
		if len(roundTripped.Steps) != len(original.Steps) {
			t.Fatalf("len(Steps): got %d, want %d", len(roundTripped.Steps), len(original.Steps))
		}

		for i, origStep := range original.Steps {
			rtStep := roundTripped.Steps[i]
			if rtStep.Action != origStep.Action {
				t.Errorf("Steps[%d].Action: got %q, want %q", i, rtStep.Action, origStep.Action)
			}
			if rtStep.Evaluable != origStep.Evaluable {
				t.Errorf("Steps[%d].Evaluable: got %v, want %v", i, rtStep.Evaluable, origStep.Evaluable)
			}
			if rtStep.Deterministic != origStep.Deterministic {
				t.Errorf("Steps[%d].Deterministic: got %v, want %v", i, rtStep.Deterministic, origStep.Deterministic)
			}
			if rtStep.URL != origStep.URL {
				t.Errorf("Steps[%d].URL: got %q, want %q", i, rtStep.URL, origStep.URL)
			}
			if rtStep.Checksum != origStep.Checksum {
				t.Errorf("Steps[%d].Checksum: got %q, want %q", i, rtStep.Checksum, origStep.Checksum)
			}
			if rtStep.Size != origStep.Size {
				t.Errorf("Steps[%d].Size: got %d, want %d", i, rtStep.Size, origStep.Size)
			}
			if !reflect.DeepEqual(rtStep.Params, origStep.Params) {
				t.Errorf("Steps[%d].Params: got %v, want %v", i, rtStep.Params, origStep.Params)
			}
		}
	})

	t.Run("storage to executor to storage preserves data", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		original := &install.Plan{
			FormatVersion: 1,
			Tool:          "terraform",
			Version:       "1.6.0",
			Platform: install.PlanPlatform{
				OS:   "darwin",
				Arch: "amd64",
			},
			GeneratedAt:   now,
			RecipeHash:    "xyz789",
			RecipeSource:  "registry",
			Deterministic: true,
			Steps: []install.PlanStep{
				{
					Action:        "download",
					Params:        map[string]interface{}{"url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_darwin_amd64.zip"},
					Evaluable:     true,
					Deterministic: true,
					URL:           "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_darwin_amd64.zip",
					Checksum:      "sha256:123abc",
					Size:          25000000,
				},
			},
		}

		// Convert to executor format and back
		executor := FromStoragePlan(original)
		roundTripped := ToStoragePlan(executor)

		// Compare top-level fields
		if roundTripped.FormatVersion != original.FormatVersion {
			t.Errorf("FormatVersion: got %d, want %d", roundTripped.FormatVersion, original.FormatVersion)
		}
		if roundTripped.Tool != original.Tool {
			t.Errorf("Tool: got %q, want %q", roundTripped.Tool, original.Tool)
		}
		if roundTripped.Version != original.Version {
			t.Errorf("Version: got %q, want %q", roundTripped.Version, original.Version)
		}
		if roundTripped.Platform != original.Platform {
			t.Errorf("Platform: got %+v, want %+v", roundTripped.Platform, original.Platform)
		}
		if !roundTripped.GeneratedAt.Equal(original.GeneratedAt) {
			t.Errorf("GeneratedAt: got %v, want %v", roundTripped.GeneratedAt, original.GeneratedAt)
		}
		if roundTripped.RecipeHash != original.RecipeHash {
			t.Errorf("RecipeHash: got %q, want %q", roundTripped.RecipeHash, original.RecipeHash)
		}
		if roundTripped.RecipeSource != original.RecipeSource {
			t.Errorf("RecipeSource: got %q, want %q", roundTripped.RecipeSource, original.RecipeSource)
		}
		if roundTripped.Deterministic != original.Deterministic {
			t.Errorf("Deterministic: got %v, want %v", roundTripped.Deterministic, original.Deterministic)
		}

		// Compare steps
		if len(roundTripped.Steps) != len(original.Steps) {
			t.Fatalf("len(Steps): got %d, want %d", len(roundTripped.Steps), len(original.Steps))
		}

		for i, origStep := range original.Steps {
			rtStep := roundTripped.Steps[i]
			if rtStep.Action != origStep.Action {
				t.Errorf("Steps[%d].Action: got %q, want %q", i, rtStep.Action, origStep.Action)
			}
			if rtStep.Evaluable != origStep.Evaluable {
				t.Errorf("Steps[%d].Evaluable: got %v, want %v", i, rtStep.Evaluable, origStep.Evaluable)
			}
			if rtStep.Deterministic != origStep.Deterministic {
				t.Errorf("Steps[%d].Deterministic: got %v, want %v", i, rtStep.Deterministic, origStep.Deterministic)
			}
			if rtStep.URL != origStep.URL {
				t.Errorf("Steps[%d].URL: got %q, want %q", i, rtStep.URL, origStep.URL)
			}
			if rtStep.Checksum != origStep.Checksum {
				t.Errorf("Steps[%d].Checksum: got %q, want %q", i, rtStep.Checksum, origStep.Checksum)
			}
			if rtStep.Size != origStep.Size {
				t.Errorf("Steps[%d].Size: got %d, want %d", i, rtStep.Size, origStep.Size)
			}
			if !reflect.DeepEqual(rtStep.Params, origStep.Params) {
				t.Errorf("Steps[%d].Params: got %v, want %v", i, rtStep.Params, origStep.Params)
			}
		}
	})
}
