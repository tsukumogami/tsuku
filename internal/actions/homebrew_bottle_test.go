package actions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/bottletag"
)

// manifestEntries builds manifest entries for a version, one per tag,
// with the tag doubling as the blob digest so a test can see which entry
// was picked.
func manifestEntries(version string, tags ...string) []ghcrManifestEntry {
	var entries []ghcrManifestEntry
	for _, tag := range tags {
		entries = append(entries, ghcrManifestEntry{Annotations: map[string]string{
			"org.opencontainers.image.ref.name": version + "." + tag,
			"sh.brew.bottle.digest":             "sha256:" + tag,
		}})
	}
	return entries
}

func TestPickBottle(t *testing.T) {
	t.Parallel()
	// readline 8.3.6's shape: no Sonoma bottle, in no particular order.
	noSonoma := manifestEntries("8.3.6", "arm64_tahoe", "x86_64_linux", "arm64_sequoia", "arm64_golden_gate", "arm64_linux")
	// pkgconf's shape: Sonoma still present.
	withSonoma := manifestEntries("3.0.7", "arm64_tahoe", "arm64_sequoia", "arm64_sonoma", "x86_64_linux")
	onlyTahoe := manifestEntries("1.0", "arm64_tahoe", "arm64_golden_gate", "x86_64_linux")

	cases := []struct {
		name    string
		entries []ghcrManifestEntry
		version string
		ceiling int
		want    string
		wantErr string
	}{
		{"no Sonoma bottle, plan for another machine", noSonoma, "8.3.6", 0, "arm64_sequoia", ""},
		{"no Sonoma bottle, Sequoia host", noSonoma, "8.3.6", 15, "arm64_sequoia", ""},
		{"no Sonoma bottle, Tahoe host", noSonoma, "8.3.6", 26, "arm64_sequoia", ""},
		{"no Sonoma bottle, Sonoma host", noSonoma, "8.3.6", 14, "", "no compatible bottle"},
		{"Sonoma bottle kept on a Tahoe host", withSonoma, "3.0.7", 26, "arm64_sonoma", ""},
		{"Sonoma bottle kept across machines", withSonoma, "3.0.7", 0, "arm64_sonoma", ""},
		{"Tahoe-only formula on a Tahoe host", onlyTahoe, "1.0", 26, "arm64_tahoe", ""},
		{"Tahoe-only formula across machines", onlyTahoe, "1.0", 0, "", "no compatible bottle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidates, err := bottletag.Candidates("darwin", "arm64", tc.ceiling)
			if err != nil {
				t.Fatal(err)
			}
			sha, tag, err := pickBottle(tc.entries, tc.version, candidates)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("pickBottle: %v", err)
			}
			if tag != tc.want || sha != tc.want {
				t.Errorf("picked tag %q (blob %q), want %q", tag, sha, tc.want)
			}
		})
	}
}

func TestPickBottle_LinuxErrorUnchanged(t *testing.T) {
	t.Parallel()
	_, _, err := pickBottle(manifestEntries("1.0", "arm64_sonoma"), "1.0", []string{"x86_64_linux"})
	if err == nil || !strings.Contains(err.Error(), "no bottle found for platform tag: x86_64_linux") {
		t.Errorf("err = %v", err)
	}
}

// it picks and the bottle_tag it records on the relocate step.
func TestHomebrewAction_Decompose_RecordsBottleTag(t *testing.T) {
	entries := manifestEntries("8.3.6", "arm64_tahoe", "arm64_sequoia", "arm64_golden_gate", "x86_64_linux")
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
	})
	mux.HandleFunc("/api/formula/readline.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"revision": 0})
	})
	mux.HandleFunc("/v2/homebrew/core/readline/manifests/8.3.6", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"manifests": entries})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	oldGHCR, oldAPI := ghcrBaseURL, formulaeAPIBaseURL
	ghcrBaseURL, formulaeAPIBaseURL = srv.URL, srv.URL
	defer func() { ghcrBaseURL, formulaeAPIBaseURL = oldGHCR, oldAPI }()

	cases := []struct {
		name         string
		os, arch     string
		macOSVersion int
		wantBlob     string
		wantTag      string // "" means no bottle_tag param
	}{
		{"darwin plan for another machine", "darwin", "arm64", 0, "arm64_sequoia", "arm64_sequoia"},
		{"darwin plan for a Tahoe host", "darwin", "arm64", 26, "arm64_sequoia", "arm64_sequoia"},
		{"linux plan", "linux", "amd64", 0, "x86_64_linux", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &EvalContext{
				Context:      context.Background(),
				VersionTag:   "8.3.6",
				OS:           tc.os,
				Arch:         tc.arch,
				MacOSVersion: tc.macOSVersion,
			}
			steps, err := (&HomebrewAction{}).Decompose(ctx, map[string]interface{}{"formula": "readline"})
			if err != nil {
				t.Fatalf("Decompose: %v", err)
			}
			if len(steps) != 3 {
				t.Fatalf("got %d steps, want 3", len(steps))
			}
			if steps[0].Checksum != tc.wantBlob {
				t.Errorf("download checksum = %q, want %q", steps[0].Checksum, tc.wantBlob)
			}
			tag, has := steps[2].Params["bottle_tag"]
			if tc.wantTag == "" {
				if has {
					t.Errorf("relocate step has bottle_tag %v, want none", tag)
				}
				return
			}
			if tag != tc.wantTag {
				t.Errorf("relocate bottle_tag = %v, want %q", tag, tc.wantTag)
			}
		})
	}
}
