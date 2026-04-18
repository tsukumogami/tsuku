package containerimages

import (
	"encoding/json"
	"testing"
)

func TestImageForFamily_KnownFamilies(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"debian": "docker.io/library/debian:bookworm-slim@sha256:98f4b71de414932439ac6ac690d7060df1f27161073c5036a7553723881bffbe",
		"rhel":   "docker.io/library/fedora:41@sha256:f1a3fab47bcb3c3ddf3135d5ee7ba8b7b25f2e809a47440936212a3a50957f3d",
		"arch":   "docker.io/library/archlinux:base@sha256:e25a13ea0e2a36df12f3593fe4bc1063605cfd2ab46c704f72c9e1c3514138ce",
		"alpine": "docker.io/library/alpine:3.21@sha256:c3f8e73fdb79deaebaa2037150150191b9dcbfba68b4a46d70103204c53f4709",
		"suse":   "docker.io/opensuse/leap:15.6@sha256:045fc29f76266cd8176906ab1d63fcd0f505fe1182c06398631effa8f55e10d0",
	}

	for family, wantImage := range expected {
		t.Run(family, func(t *testing.T) {
			t.Parallel()

			got, ok := ImageForFamily(family)
			if !ok {
				t.Fatalf("ImageForFamily(%q) returned ok=false, want true", family)
			}
			if got != wantImage {
				t.Errorf("ImageForFamily(%q) = %q, want %q", family, got, wantImage)
			}
		})
	}
}

func TestImageForFamily_UnknownFamily(t *testing.T) {
	t.Parallel()

	unknowns := []string{"ubuntu", "centos", "gentoo", "", "DEBIAN"}
	for _, family := range unknowns {
		t.Run(family, func(t *testing.T) {
			t.Parallel()

			img, ok := ImageForFamily(family)
			if ok {
				t.Errorf("ImageForFamily(%q) returned ok=true with image %q, want ok=false", family, img)
			}
			if img != "" {
				t.Errorf("ImageForFamily(%q) = %q, want empty string", family, img)
			}
		})
	}
}

func TestDefaultImage(t *testing.T) {
	t.Parallel()

	got := DefaultImage()
	if got != "docker.io/library/debian:bookworm-slim@sha256:98f4b71de414932439ac6ac690d7060df1f27161073c5036a7553723881bffbe" {
		t.Errorf("DefaultImage() = %q, want %q", got, "docker.io/library/debian:bookworm-slim@sha256:98f4b71de414932439ac6ac690d7060df1f27161073c5036a7553723881bffbe")
	}
}

func TestEmbeddedJSON_Valid(t *testing.T) {
	t.Parallel()

	var parsed map[string]familyConfig
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		t.Fatalf("embedded container-images.json is not valid JSON: %v", err)
	}

	requiredFamilies := []string{"debian", "rhel", "arch", "alpine", "suse"}
	for _, family := range requiredFamilies {
		cfg, ok := parsed[family]
		if !ok {
			t.Errorf("embedded JSON missing required family %q", family)
			continue
		}
		if cfg.Image == "" {
			t.Errorf("family %q has empty image string", family)
		}
		for _, cat := range []string{"core", "network", "build"} {
			if _, ok := cfg.InfraPackages[cat]; !ok {
				t.Errorf("family %q missing infra_packages.%s", family, cat)
			}
		}
	}
}

func TestEmbeddedJSON_AllEntriesNonEmpty(t *testing.T) {
	t.Parallel()

	var parsed map[string]familyConfig
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		t.Fatalf("embedded JSON parse error: %v", err)
	}

	for family, cfg := range parsed {
		if cfg.Image == "" {
			t.Errorf("family %q has empty image string", family)
		}
	}
}

func TestFamilies(t *testing.T) {
	t.Parallel()

	fams := Families()
	if len(fams) != 5 {
		t.Errorf("Families() returned %d families, want 5", len(fams))
	}

	famSet := make(map[string]bool)
	for _, f := range fams {
		famSet[f] = true
	}

	required := []string{"debian", "rhel", "arch", "alpine", "suse"}
	for _, r := range required {
		if !famSet[r] {
			t.Errorf("Families() missing %q", r)
		}
	}
}

func TestInfraPackages_KnownCategory(t *testing.T) {
	t.Parallel()

	pkgs := InfraPackages("suse", "core")
	expected := []string{"tar", "gzip", "zstd"}
	if len(pkgs) != len(expected) {
		t.Fatalf("InfraPackages(suse, core) = %v, want %v", pkgs, expected)
	}
	for i, p := range expected {
		if pkgs[i] != p {
			t.Errorf("InfraPackages(suse, core)[%d] = %q, want %q", i, pkgs[i], p)
		}
	}
}

func TestInfraPackages_EmptyCategory(t *testing.T) {
	t.Parallel()

	// debian core is an empty array in the JSON, should return nil
	pkgs := InfraPackages("debian", "core")
	if pkgs != nil {
		t.Errorf("InfraPackages(debian, core) = %v, want nil", pkgs)
	}
}

func TestInfraPackages_UnknownFamily(t *testing.T) {
	t.Parallel()

	pkgs := InfraPackages("gentoo", "network")
	if pkgs != nil {
		t.Errorf("InfraPackages(gentoo, network) = %v, want nil", pkgs)
	}
}

func TestInfraPackages_AllFamiliesHaveNetworkPackages(t *testing.T) {
	t.Parallel()

	for _, family := range Families() {
		pkgs := InfraPackages(family, "network")
		if pkgs == nil {
			t.Errorf("InfraPackages(%q, network) = nil, want non-nil", family)
			continue
		}
		has := make(map[string]bool)
		for _, p := range pkgs {
			has[p] = true
		}
		for _, required := range []string{"ca-certificates", "curl"} {
			if !has[required] {
				t.Errorf("InfraPackages(%q, network) missing %q", family, required)
			}
		}
	}
}

func TestInfraPackages_ReturnsCopy(t *testing.T) {
	t.Parallel()

	pkgs := InfraPackages("suse", "core")
	if len(pkgs) == 0 {
		t.Skip("no core packages for suse")
	}
	// Mutate the returned slice
	pkgs[0] = "MUTATED"

	// Fetch again — should be unchanged
	pkgs2 := InfraPackages("suse", "core")
	if pkgs2[0] == "MUTATED" {
		t.Error("InfraPackages returned a reference to internal data, not a copy")
	}
}
