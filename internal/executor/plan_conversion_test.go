package executor

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/install"
)

// fullyPopulatedPlan is the fixture the conversion guard runs on. Every
// exported field of every type it reaches is set to something distinguishable
// from that field's zero value, which is what lets the census below turn a
// newly added field into a test failure.
//
// Params values stay within what JSON round-trips to the same Go value:
// strings, float64, bool, and slices of those. An int here would come back a
// float64 and fail the comparison for a reason that has nothing to do with the
// converter.
func fullyPopulatedPlan() *InstallationPlan {
	exitCode := 3
	return &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "kubectl",
		Version:       "1.29.0",
		Platform: Platform{
			OS:          "linux",
			Arch:        "arm64",
			LinuxFamily: "debian",
		},
		GeneratedAt:   time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		RecipeSource:  "registry",
		Deterministic: true,
		Dependencies: []DependencyPlan{
			{
				Tool:    "openssl",
				Version: "3.2.1",
				Dependencies: []DependencyPlan{
					{
						Tool:       "ca-certificates",
						Version:    "2024-03-11",
						RecipeType: "library",
						Steps: []ResolvedStep{
							{
								Action:        "download_file",
								Phase:         "install",
								Params:        map[string]interface{}{"url": "https://example.com/ca.tar.gz"},
								Evaluable:     true,
								Deterministic: true,
								URL:           "https://example.com/ca.tar.gz",
								Checksum:      "b1946ac92492d2347c6235b4d2611184",
								Size:          4096,
							},
						},
						Verify: &PlanVerify{
							Command:  "openssl version",
							Pattern:  "OpenSSL",
							Patterns: []string{"OpenSSL", "3.2"},
							ExitCode: &exitCode,
							Additional: []PlanAdditionalVerify{
								{Command: "openssl help", Pattern: "usage"},
							},
						},
					},
				},
				RecipeType: "library",
				Steps: []ResolvedStep{
					{
						Action:        "extract",
						Phase:         "install",
						Params:        map[string]interface{}{"format": "tar.gz", "strip": float64(1)},
						Evaluable:     true,
						Deterministic: true,
						URL:           "https://example.com/openssl.tar.gz",
						Checksum:      "5eb63bbbe01eeed093cb22bb8f5acdc3",
						Size:          8192,
					},
				},
				Verify: &PlanVerify{
					Command:  "openssl version",
					Pattern:  "OpenSSL",
					Patterns: []string{"OpenSSL"},
					ExitCode: &exitCode,
					Additional: []PlanAdditionalVerify{
						{Command: "openssl ciphers", Pattern: "AES"},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action:        "download_file",
				Phase:         "post-install",
				Params:        map[string]interface{}{"url": "https://example.com/kubectl", "executable": true},
				Evaluable:     true,
				Deterministic: true,
				URL:           "https://example.com/kubectl",
				Checksum:      "9e107d9d372bb6826bd81d3542a419d6",
				Size:          50000000,
			},
		},
		Verify: &PlanVerify{
			Command:  "kubectl version --client",
			Pattern:  "Client Version",
			Patterns: []string{"Client Version", "1.29"},
			ExitCode: &exitCode,
			Additional: []PlanAdditionalVerify{
				{Command: "kubectl config view", Pattern: "apiVersion"},
			},
		},
		RecipeType: "tool",
		Binaries:   []string{"bin/kubectl"},
	}
}

// TestPlanConversionCarriesEveryField is the guard that the field-by-field
// assertions this replaced could not be. It has two halves, and neither works
// alone.
//
// The census proves the fixture is complete: add a field to InstallationPlan,
// ResolvedStep, Platform, DependencyPlan, PlanVerify, or PlanAdditionalVerify
// and it fails, naming the field, before anyone has looked at the converter.
//
// The round trip proves the converter and the storage types carry what the
// fixture holds. It goes through JSON because that is what state.json does --
// without the encode-decode hop, a field missing from install.Plan but present
// in the converter's field list would slip through.
func TestPlanConversionCarriesEveryField(t *testing.T) {
	original := fullyPopulatedPlan()

	t.Run("fixture leaves no exported field at its zero value", func(t *testing.T) {
		assertFullyPopulated(t, reflect.ValueOf(original), "InstallationPlan", map[reflect.Type]bool{})
	})

	t.Run("executor to storage to JSON to executor preserves everything", func(t *testing.T) {
		stored := ToStoragePlan(original)

		encoded, err := json.Marshal(stored)
		if err != nil {
			t.Fatalf("marshal stored plan: %v", err)
		}

		var decoded install.Plan
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal stored plan: %v", err)
		}

		got := FromStoragePlan(&decoded)
		if !reflect.DeepEqual(got, original) {
			t.Errorf("round trip changed the plan.\n got: %+v\nwant: %+v", got, original)
		}
	})
}

// assertFullyPopulated walks a value and reports every exported field left at
// its zero value.
//
// The seen set holds the struct types on the current path so a self-recursive
// type terminates: DependencyPlan nested in DependencyPlan is checked once,
// which is enough to prove the recursion is populated without demanding an
// infinitely deep fixture.
func assertFullyPopulated(t *testing.T, v reflect.Value, path string, seen map[reflect.Type]bool) {
	t.Helper()

	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			t.Errorf("%s is nil; the fixture must populate every field", path)
			return
		}
		assertFullyPopulated(t, v.Elem(), path, seen)

	case reflect.Struct:
		// time.Time has unexported fields; treat it as a leaf.
		if v.Type() == reflect.TypeOf(time.Time{}) {
			if v.Interface().(time.Time).IsZero() {
				t.Errorf("%s is the zero time; the fixture must populate every field", path)
			}
			return
		}
		if seen[v.Type()] {
			return // one level of a self-recursive type is enough
		}
		seen[v.Type()] = true
		defer delete(seen, v.Type())

		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			assertFullyPopulated(t, v.Field(i), path+"."+field.Name, seen)
		}

	case reflect.Slice, reflect.Map:
		if v.Len() == 0 {
			t.Errorf("%s is empty; the fixture must populate every field", path)
			return
		}
		if v.Kind() == reflect.Slice {
			assertFullyPopulated(t, v.Index(0), path+"[0]", seen)
		}
		// Map values are interface{} params. Their contents are not part of
		// the plan's field surface.

	case reflect.Interface:
		// Params values. Nothing to enforce beyond the map being non-empty.

	default:
		if v.IsZero() {
			t.Errorf("%s is the zero %s; the fixture must populate every field", path, v.Kind())
		}
	}
}

// TestRoundTripPreservesPreviouslyDroppedFields names the fields ToStoragePlan
// used to discard. The census above catches a regression in any of them, but
// only as "the round trip changed the plan". This says which field and why it
// matters.
func TestRoundTripPreservesPreviouslyDroppedFields(t *testing.T) {
	got := FromStoragePlan(ToStoragePlan(fullyPopulatedPlan()))

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"step phase routes post-install steps", got.Steps[0].Phase, "post-install"},
		{"dependency tree, the only source of deps on the --plan path", len(got.Dependencies), 1},
		{"nested dependency tree", len(got.Dependencies[0].Dependencies), 1},
		{"verify block, whose absence skips verification silently", got.Verify.Command, "kubectl version --client"},
		{"verify additional commands", len(got.Verify.Additional), 1},
		{"dependency verify block", got.Dependencies[0].Verify.Command, "openssl version"},
		{"recipe type, which decides library handling", got.Dependencies[0].RecipeType, "library"},
		{"binaries, the fallback when no install_binaries step exists", len(got.Binaries), 1},
		{"linux family targeting", got.Platform.LinuxFamily, "debian"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestToStoragePlan(t *testing.T) {
	t.Run("nil plan returns nil", func(t *testing.T) {
		if result := ToStoragePlan(nil); result != nil {
			t.Errorf("ToStoragePlan(nil) = %v, want nil", result)
		}
	})

	t.Run("stamps the current storage version", func(t *testing.T) {
		stored := ToStoragePlan(fullyPopulatedPlan())
		if stored.StorageVersion != install.PlanStorageVersion {
			t.Errorf("StorageVersion = %d, want %d", stored.StorageVersion, install.PlanStorageVersion)
		}
	})

	t.Run("a plan with nothing optional to say stays lean", func(t *testing.T) {
		// The added fields are all omitempty, so a minimal plan must not grow
		// new keys. A rewritten state.json record should differ from what the
		// old conversion wrote only by storage_version.
		stored := ToStoragePlan(&InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "jq",
			Version:       "1.7",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Steps:         []ResolvedStep{{Action: "chmod", Params: map[string]interface{}{"path": "bin/jq"}}},
		})
		encoded, err := json.Marshal(stored)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &keys); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, absent := range []string{"dependencies", "verify", "recipe_type", "binaries", "linux_family", "phase"} {
			if _, present := keys[absent]; present {
				t.Errorf("key %q written for a plan that has none", absent)
			}
		}
	})
}

func TestFromStoragePlan(t *testing.T) {
	t.Run("nil plan returns nil", func(t *testing.T) {
		if result := FromStoragePlan(nil); result != nil {
			t.Errorf("FromStoragePlan(nil) = %v, want nil", result)
		}
	})

	t.Run("a record written before the fields existed decodes to their zero values", func(t *testing.T) {
		// What a pre-fix state.json record looks like on disk. The converter
		// restores what is there and nothing more; deciding what the absence
		// means belongs to the read sites, which gate on StorageVersion.
		const preFix = `{
			"format_version": 5,
			"tool": "gh",
			"version": "2.40.0",
			"platform": {"os": "linux", "arch": "amd64"},
			"recipe_source": "registry",
			"deterministic": true,
			"steps": [{"action": "chmod", "params": {"path": "bin/gh"}, "evaluable": true, "deterministic": true}]
		}`

		var stored install.Plan
		if err := json.Unmarshal([]byte(preFix), &stored); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if stored.StorageVersion != 0 {
			t.Errorf("StorageVersion = %d, want 0 for a pre-fix record", stored.StorageVersion)
		}

		plan := FromStoragePlan(&stored)
		if plan.Dependencies != nil {
			t.Errorf("Dependencies = %v, want nil", plan.Dependencies)
		}
		if plan.Verify != nil {
			t.Errorf("Verify = %v, want nil", plan.Verify)
		}
		if plan.Steps[0].Phase != "" {
			t.Errorf("Steps[0].Phase = %q, want empty", plan.Steps[0].Phase)
		}
	})
}
