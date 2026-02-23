package containerimages

import (
	"encoding/json"
	"testing"
)

func TestImageForFamily_KnownFamilies(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"debian": "debian:bookworm-slim",
		"rhel":   "fedora:41",
		"arch":   "archlinux:base",
		"alpine": "alpine:3.21",
		"suse":   "opensuse/tumbleweed",
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
	if got != "debian:bookworm-slim" {
		t.Errorf("DefaultImage() = %q, want %q", got, "debian:bookworm-slim")
	}
}

func TestEmbeddedJSON_Valid(t *testing.T) {
	t.Parallel()

	var parsed map[string]string
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		t.Fatalf("embedded container-images.json is not valid JSON: %v", err)
	}

	requiredFamilies := []string{"debian", "rhel", "arch", "alpine", "suse"}
	for _, family := range requiredFamilies {
		if _, ok := parsed[family]; !ok {
			t.Errorf("embedded JSON missing required family %q", family)
		}
	}
}

func TestEmbeddedJSON_AllEntriesNonEmpty(t *testing.T) {
	t.Parallel()

	var parsed map[string]string
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		t.Fatalf("embedded JSON parse error: %v", err)
	}

	for family, image := range parsed {
		if image == "" {
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
