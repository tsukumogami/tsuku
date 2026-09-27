package bottletag

import (
	"reflect"
	"strings"
	"testing"
)

func TestCandidates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		os      string
		arch    string
		ceiling int
		want    []string
		wantErr bool
	}{
		{"linux amd64", "linux", "amd64", 0, []string{"x86_64_linux"}, false},
		{"linux arm64", "linux", "arm64", 26, []string{"arm64_linux"}, false},
		{
			"cross-machine arm64 stops at Sequoia", "darwin", "arm64", 0,
			[]string{"arm64_sonoma", "arm64_sequoia", "arm64_ventura", "arm64_monterey", "arm64_big_sur"}, false,
		},
		{
			"Tahoe host walks up to Tahoe", "darwin", "arm64", 26,
			[]string{"arm64_sonoma", "arm64_sequoia", "arm64_tahoe", "arm64_ventura", "arm64_monterey", "arm64_big_sur"}, false,
		},
		{
			"Sonoma host never goes above Sonoma", "darwin", "arm64", 14,
			[]string{"arm64_sonoma", "arm64_ventura", "arm64_monterey", "arm64_big_sur"}, false,
		},
		{
			"Ventura host starts below Sonoma", "darwin", "arm64", 13,
			[]string{"arm64_ventura", "arm64_monterey", "arm64_big_sur"}, false,
		},
		{
			"host newer than every known release", "darwin", "arm64", 30,
			[]string{"arm64_sonoma", "arm64_sequoia", "arm64_tahoe", "arm64_golden_gate", "arm64_ventura", "arm64_monterey", "arm64_big_sur"}, false,
		},
		{
			"Intel tags have no arch prefix", "darwin", "amd64", 0,
			[]string{"sonoma", "sequoia", "ventura", "monterey", "big_sur"}, false,
		},
		{"macOS older than any bottle", "darwin", "arm64", 10, nil, true},
		{"unsupported os", "windows", "amd64", 0, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Candidates(tc.os, tc.arch, tc.ceiling)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Candidates(%s, %s, %d) = %v, want %v", tc.os, tc.arch, tc.ceiling, got, tc.want)
			}
		})
	}
}

func TestParseMacOSVersion(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"15.6.1":  15,
		"14.0":    14,
		"26.0.1":  26,
		"16.0":    26, // compatibility value seen on macOS 26 betas
		"10.16":   11, // compatibility value on Big Sur
		"10.15.7": 10,
		"27.0\n":  27,
		"":        0,
		"abc":     0,
	}
	for in, want := range cases {
		if got := parseMacOSVersion(in); got != want {
			t.Errorf("parseMacOSVersion(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestMacOSVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag     string
		version int
		arch    string
		ok      bool
	}{
		{"arm64_sonoma", 14, "arm64", true},
		{"sequoia", 15, "amd64", true},
		{"arm64_tahoe", 26, "arm64", true},
		{"arm64_golden_gate", 27, "arm64", true},
		{"x86_64_linux", 0, "", false},
		{"arm64_linux", 0, "", false},
		{"arm64_someday", 0, "", false},
	}
	for _, tc := range cases {
		v, arch, ok := MacOSVersion(tc.tag)
		if v != tc.version || arch != tc.arch || ok != tc.ok {
			t.Errorf("MacOSVersion(%q) = (%d, %q, %v), want (%d, %q, %v)", tc.tag, v, arch, ok, tc.version, tc.arch, tc.ok)
		}
	}
}

func TestCheckForHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag     string
		host    int
		wantErr string
	}{
		{"arm64_tahoe", 15, "built for macOS 26, but this system runs macOS 15"},
		{"arm64_sequoia", 14, "built for macOS 15, but this system runs macOS 14"},
		{"arm64_sequoia", 15, ""},
		{"arm64_sonoma", 26, ""},
		{"arm64_tahoe", 26, ""},
		{"x86_64_linux", 15, ""},
		{"arm64_tahoe", 0, ""}, // host version unknown
		{"arm64_someday", 26, "doesn't know"},
	}
	for _, tc := range cases {
		err := CheckForHost(tc.tag, tc.host)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("CheckForHost(%q, %d) = %v, want nil", tc.tag, tc.host, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("CheckForHost(%q, %d) = %v, want error containing %q", tc.tag, tc.host, err, tc.wantErr)
		}
	}
}

// TestHomebrewAction_Decompose_RecordsBottleTag runs Decompose against a
// local stand-in for GHCR and formulae.brew.sh and checks both the bottle
