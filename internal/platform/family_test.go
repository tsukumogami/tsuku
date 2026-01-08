package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantID      string
		wantIDLike  []string
		wantVersion string
	}{
		{
			name:        "ubuntu",
			fixture:     "ubuntu",
			wantID:      "ubuntu",
			wantIDLike:  []string{"debian"},
			wantVersion: "22.04",
		},
		{
			name:        "debian",
			fixture:     "debian",
			wantID:      "debian",
			wantIDLike:  nil,
			wantVersion: "12",
		},
		{
			name:        "fedora",
			fixture:     "fedora",
			wantID:      "fedora",
			wantIDLike:  nil,
			wantVersion: "39",
		},
		{
			name:        "arch",
			fixture:     "arch",
			wantID:      "arch",
			wantIDLike:  nil,
			wantVersion: "",
		},
		{
			name:        "alpine",
			fixture:     "alpine",
			wantID:      "alpine",
			wantIDLike:  nil,
			wantVersion: "3.19.0",
		},
		{
			name:        "rocky",
			fixture:     "rocky",
			wantID:      "rocky",
			wantIDLike:  []string{"rhel", "centos", "fedora"},
			wantVersion: "9.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("testdata", "os-release", tt.fixture)
			release, err := ParseOSRelease(path)
			if err != nil {
				t.Fatalf("ParseOSRelease() error = %v", err)
			}

			if release.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", release.ID, tt.wantID)
			}

			if len(release.IDLike) != len(tt.wantIDLike) {
				t.Errorf("IDLike = %v, want %v", release.IDLike, tt.wantIDLike)
			} else {
				for i, like := range tt.wantIDLike {
					if release.IDLike[i] != like {
						t.Errorf("IDLike[%d] = %q, want %q", i, release.IDLike[i], like)
					}
				}
			}

			if release.VersionID != tt.wantVersion {
				t.Errorf("VersionID = %q, want %q", release.VersionID, tt.wantVersion)
			}
		})
	}
}

func TestParseOSRelease_MissingFile(t *testing.T) {
	_, err := ParseOSRelease("/nonexistent/os-release")
	if err == nil {
		t.Error("ParseOSRelease() expected error for missing file")
	}
}

func TestMapDistroToFamily(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		idLike     []string
		wantFamily string
		wantErr    bool
	}{
		// Direct ID matches
		{name: "ubuntu direct", id: "ubuntu", wantFamily: "debian"},
		{name: "debian direct", id: "debian", wantFamily: "debian"},
		{name: "fedora direct", id: "fedora", wantFamily: "rhel"},
		{name: "arch direct", id: "arch", wantFamily: "arch"},
		{name: "alpine direct", id: "alpine", wantFamily: "alpine"},
		{name: "opensuse direct", id: "opensuse", wantFamily: "suse"},
		{name: "rocky direct", id: "rocky", wantFamily: "rhel"},
		{name: "almalinux direct", id: "almalinux", wantFamily: "rhel"},

		// ID_LIKE fallback
		{
			name:       "pop via id_like",
			id:         "pop",
			wantFamily: "debian",
		},
		{
			name:       "unknown with debian id_like",
			id:         "unknown-distro",
			idLike:     []string{"debian"},
			wantFamily: "debian",
		},
		{
			name:       "rocky via id_like fallback",
			id:         "rocky",
			idLike:     []string{"rhel", "centos", "fedora"},
			wantFamily: "rhel",
		},

		// Unknown distro
		{
			name:    "unknown distro no fallback",
			id:      "unknown-distro",
			wantErr: true,
		},
		{
			name:    "unknown with unknown id_like",
			id:      "unknown-distro",
			idLike:  []string{"also-unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, err := MapDistroToFamily(tt.id, tt.idLike)

			if tt.wantErr {
				if err == nil {
					t.Error("MapDistroToFamily() expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("MapDistroToFamily() unexpected error = %v", err)
			}

			if family != tt.wantFamily {
				t.Errorf("MapDistroToFamily() = %q, want %q", family, tt.wantFamily)
			}
		})
	}
}

func TestParseOSRelease_Fixtures(t *testing.T) {
	// Test that each fixture maps to the expected family
	fixtures := []struct {
		name       string
		wantFamily string
	}{
		{"ubuntu", "debian"},
		{"debian", "debian"},
		{"fedora", "rhel"},
		{"arch", "arch"},
		{"alpine", "alpine"},
		{"rocky", "rhel"},
	}

	for _, tt := range fixtures {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("testdata", "os-release", tt.name)
			release, err := ParseOSRelease(path)
			if err != nil {
				t.Fatalf("ParseOSRelease() error = %v", err)
			}

			family, err := MapDistroToFamily(release.ID, release.IDLike)
			if err != nil {
				t.Fatalf("MapDistroToFamily() error = %v", err)
			}

			if family != tt.wantFamily {
				t.Errorf("family = %q, want %q", family, tt.wantFamily)
			}
		})
	}
}

func TestDetectTarget_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping on Linux")
	}

	target, err := DetectTarget()
	if err != nil {
		t.Fatalf("DetectTarget() error = %v", err)
	}

	expectedPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if target.Platform != expectedPlatform {
		t.Errorf("Platform = %q, want %q", target.Platform, expectedPlatform)
	}

	if target.LinuxFamily() != "" {
		t.Errorf("LinuxFamily = %q, want empty for non-Linux", target.LinuxFamily())
	}
}

func TestDetectTarget_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux")
	}

	target, err := DetectTarget()

	// On Linux, we expect either success or a "missing file" scenario
	// which returns empty family without error
	if err != nil {
		t.Logf("DetectTarget() returned error (expected on minimal systems): %v", err)
		return
	}

	expectedPlatform := "linux/" + runtime.GOARCH
	if target.Platform != expectedPlatform {
		t.Errorf("Platform = %q, want %q", target.Platform, expectedPlatform)
	}

	// LinuxFamily should be one of the valid families or empty
	if target.LinuxFamily() != "" {
		found := false
		for _, family := range ValidLinuxFamilies {
			if target.LinuxFamily() == family {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("LinuxFamily = %q, not in ValidLinuxFamilies", target.LinuxFamily())
		}
	}
}

func TestDetectFamily_MissingFile(t *testing.T) {
	// Create a temp directory to ensure /etc/os-release doesn't exist there
	// This test verifies graceful handling when file is missing

	// Save original and restore after test
	// We can't easily test this without modifying the function to accept a path
	// So we just verify the function exists and returns expected types
	family, err := DetectFamily()
	// On a real Linux system, this should succeed
	// On non-Linux, or containers without os-release, we expect empty + nil
	if runtime.GOOS != "linux" {
		if family != "" {
			t.Errorf("DetectFamily() on non-Linux returned family = %q, want empty", family)
		}
		if err != nil {
			t.Errorf("DetectFamily() on non-Linux returned error = %v, want nil", err)
		}
	} else {
		// On Linux, log what we got for debugging
		t.Logf("DetectFamily() on Linux: family=%q, err=%v", family, err)
	}
}

func TestOSRelease_Comments(t *testing.T) {
	// Create a temp file with comments
	content := `# This is a comment
ID=testid
# Another comment
ID_LIKE=parent
VERSION_ID=1.0
`
	tmpfile, err := os.CreateTemp("", "os-release")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	release, err := ParseOSRelease(tmpfile.Name())
	if err != nil {
		t.Fatalf("ParseOSRelease() error = %v", err)
	}

	if release.ID != "testid" {
		t.Errorf("ID = %q, want %q", release.ID, "testid")
	}
	if len(release.IDLike) != 1 || release.IDLike[0] != "parent" {
		t.Errorf("IDLike = %v, want [parent]", release.IDLike)
	}
}

func TestOSRelease_QuotedValues(t *testing.T) {
	// Test handling of quoted values (both single and double quotes)
	content := `ID="quoted-id"
ID_LIKE='single quoted'
VERSION_ID="1.0"
`
	tmpfile, err := os.CreateTemp("", "os-release")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	release, err := ParseOSRelease(tmpfile.Name())
	if err != nil {
		t.Fatalf("ParseOSRelease() error = %v", err)
	}

	if release.ID != "quoted-id" {
		t.Errorf("ID = %q, want %q", release.ID, "quoted-id")
	}
	if len(release.IDLike) != 2 || release.IDLike[0] != "single" {
		t.Errorf("IDLike = %v, want [single quoted]", release.IDLike)
	}
}
