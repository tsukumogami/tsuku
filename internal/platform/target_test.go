package platform

import "testing"

func TestTarget_OS(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{
			name:     "linux amd64",
			platform: "linux/amd64",
			want:     "linux",
		},
		{
			name:     "linux arm64",
			platform: "linux/arm64",
			want:     "linux",
		},
		{
			name:     "darwin arm64",
			platform: "darwin/arm64",
			want:     "darwin",
		},
		{
			name:     "darwin amd64",
			platform: "darwin/amd64",
			want:     "darwin",
		},
		{
			name:     "windows amd64",
			platform: "windows/amd64",
			want:     "windows",
		},
		{
			name:     "empty platform",
			platform: "",
			want:     "",
		},
		{
			name:     "no slash",
			platform: "linux",
			want:     "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := Target{Platform: tt.platform}
			if got := target.OS(); got != tt.want {
				t.Errorf("Target.OS() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTarget_Arch(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{
			name:     "linux amd64",
			platform: "linux/amd64",
			want:     "amd64",
		},
		{
			name:     "linux arm64",
			platform: "linux/arm64",
			want:     "arm64",
		},
		{
			name:     "darwin arm64",
			platform: "darwin/arm64",
			want:     "arm64",
		},
		{
			name:     "darwin amd64",
			platform: "darwin/amd64",
			want:     "amd64",
		},
		{
			name:     "windows amd64",
			platform: "windows/amd64",
			want:     "amd64",
		},
		{
			name:     "empty platform",
			platform: "",
			want:     "",
		},
		{
			name:     "no slash returns empty",
			platform: "linux",
			want:     "",
		},
		{
			name:     "trailing slash",
			platform: "linux/",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := Target{Platform: tt.platform}
			if got := target.Arch(); got != tt.want {
				t.Errorf("Target.Arch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTarget_LinuxFamily(t *testing.T) {
	tests := []struct {
		name        string
		target      Target
		wantOS      string
		wantFamily  string
		description string
	}{
		{
			name: "debian family on linux",
			target: Target{
				Platform:    "linux/amd64",
				LinuxFamily: "debian",
			},
			wantOS:      "linux",
			wantFamily:  "debian",
			description: "LinuxFamily set for Linux platform",
		},
		{
			name: "rhel family on linux",
			target: Target{
				Platform:    "linux/arm64",
				LinuxFamily: "rhel",
			},
			wantOS:      "linux",
			wantFamily:  "rhel",
			description: "LinuxFamily set for Linux platform",
		},
		{
			name: "arch family on linux",
			target: Target{
				Platform:    "linux/amd64",
				LinuxFamily: "arch",
			},
			wantOS:      "linux",
			wantFamily:  "arch",
			description: "LinuxFamily set for Linux platform",
		},
		{
			name: "alpine family on linux",
			target: Target{
				Platform:    "linux/amd64",
				LinuxFamily: "alpine",
			},
			wantOS:      "linux",
			wantFamily:  "alpine",
			description: "LinuxFamily set for Linux platform",
		},
		{
			name: "suse family on linux",
			target: Target{
				Platform:    "linux/amd64",
				LinuxFamily: "suse",
			},
			wantOS:      "linux",
			wantFamily:  "suse",
			description: "LinuxFamily set for Linux platform",
		},
		{
			name: "darwin has no family",
			target: Target{
				Platform:    "darwin/arm64",
				LinuxFamily: "",
			},
			wantOS:      "darwin",
			wantFamily:  "",
			description: "LinuxFamily empty for non-Linux",
		},
		{
			name: "windows has no family",
			target: Target{
				Platform:    "windows/amd64",
				LinuxFamily: "",
			},
			wantOS:      "windows",
			wantFamily:  "",
			description: "LinuxFamily empty for non-Linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.OS(); got != tt.wantOS {
				t.Errorf("Target.OS() = %q, want %q", got, tt.wantOS)
			}
			if got := tt.target.LinuxFamily; got != tt.wantFamily {
				t.Errorf("Target.LinuxFamily = %q, want %q", got, tt.wantFamily)
			}
		})
	}
}

func TestValidLinuxFamilies(t *testing.T) {
	expected := []string{"debian", "rhel", "arch", "alpine", "suse"}
	if len(ValidLinuxFamilies) != len(expected) {
		t.Errorf("ValidLinuxFamilies has %d entries, want %d", len(ValidLinuxFamilies), len(expected))
	}
	for i, family := range expected {
		if ValidLinuxFamilies[i] != family {
			t.Errorf("ValidLinuxFamilies[%d] = %q, want %q", i, ValidLinuxFamilies[i], family)
		}
	}
}
