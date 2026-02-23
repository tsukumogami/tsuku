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
