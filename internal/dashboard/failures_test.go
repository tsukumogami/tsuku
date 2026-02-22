package dashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestExtractSubcategory_bracketedTag(t *testing.T) {
	tests := []struct {
		name     string
		category string
		message  string
		exitCode int
		want     string
	}{
		{
			name:     "api_error tag",
			category: "deterministic_insufficient",
			message:  "deterministic generation failed: [api_error] failed to fetch bottle",
			want:     "api_error",
		},
		{
			name:     "no_bottles tag",
			category: "deterministic_insufficient",
			message:  "[no_bottles] no bottles available for this formula",
			want:     "no_bottles",
		},
		{
			name:     "complex_archive tag",
			category: "deterministic",
			message:  "deterministic generation failed: [complex_archive] unsupported layout",
			want:     "complex_archive",
		},
		{
			name:     "unknown bracketed tag ignored",
			category: "validation_failed",
			message:  "error: [unknown_tag] something happened",
			want:     "",
		},
		{
			name:     "verify_failed tag",
			category: "validation_failed",
			message:  "[verify_failed] version check failed",
			want:     "verify_failed",
		},
		{
			name:     "install_failed tag",
			category: "validation_failed",
			message:  "[install_failed] binary not found",
			want:     "install_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSubcategory(tt.category, tt.message, tt.exitCode)
			if got != tt.want {
				t.Errorf("extractSubcategory(%q, %q, %d) = %q, want %q",
					tt.category, tt.message, tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestExtractSubcategory_regexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "no bottle",
			message: "Error: No bottle available for this formula",
			want:    "no_bottle",
		},
		{
			name:    "bottle not found",
			message: "Bottle not found for linux-x86_64",
			want:    "no_bottle",
		},
		{
			name:    "no executables",
			message: "Error: no executables found in extracted archive",
			want:    "binary_discovery_failed",
		},
		{
			name:    "no binaries",
			message: "no binaries detected in installation",
			want:    "binary_discovery_failed",
		},
		{
			name:    "version pattern",
			message: "version pattern mismatch: expected v1.2.3",
			want:    "verify_pattern_mismatch",
		},
		{
			name:    "failed to verify",
			message: "failed to verify installation output",
			want:    "verify_pattern_mismatch",
		},
		{
			name:    "verification failed",
			message: "verification failed: checksum does not match",
			want:    "verify_pattern_mismatch",
		},
		{
			name:    "already exists",
			message: "Error: recipe already exists at recipes/p/pkgconf.toml",
			want:    "recipe_already_exists",
		},
		{
			name:    "use --force",
			message: "Use --force to overwrite existing recipe",
			want:    "recipe_already_exists",
		},
		{
			name:    "rate limit",
			message: "API rate limit exceeded, try again later",
			want:    "rate_limited",
		},
		{
			name:    "429 status",
			message: "HTTP 429: too many requests",
			want:    "rate_limited",
		},
		{
			name:    "5xx error",
			message: "upstream returned 5xx error",
			want:    "upstream_unavailable",
		},
		{
			name:    "unavailable",
			message: "service unavailable, please retry",
			want:    "upstream_unavailable",
		},
		{
			name:    "timeout",
			message: "connection timeout after 30s",
			want:    "timeout",
		},
		{
			name:    "deadline",
			message: "context deadline exceeded",
			want:    "timeout",
		},
		{
			name:    "recipe not found with verify suggestion",
			message: "Error: registry: recipe berkeley-db@5 not found in registry\n\nSuggestion: Verify the recipe name is correct.",
			want:    "",
		},
		{
			name:    "no match",
			message: "some other error that doesn't match patterns",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSubcategory("validation_failed", tt.message, 0)
			if got != tt.want {
				t.Errorf("extractSubcategory(_, %q, 0) = %q, want %q",
					tt.message, got, tt.want)
			}
		})
	}
}

func TestExtractSubcategory_exitCodeFallback(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		want     string
	}{
		{name: "exit code 6", exitCode: 6, want: "install_failed"},
		{name: "exit code 7", exitCode: 7, want: "verify_failed"},
		{name: "exit code 9", exitCode: 9, want: "deterministic_failed"},
		{name: "exit code 1 no match", exitCode: 1, want: ""},
		{name: "exit code 0 no match", exitCode: 0, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Exit code fallback only triggers when message is empty
			got := extractSubcategory("validation_failed", "", tt.exitCode)
			if got != tt.want {
				t.Errorf("extractSubcategory(_, \"\", %d) = %q, want %q",
					tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestExtractSubcategory_bracketPriorityOverRegex(t *testing.T) {
	// If a message has both a bracket tag and regex-matchable content,
	// the bracket tag should win.
	got := extractSubcategory("deterministic", "[api_error] no bottle available", 6)
	if got != "api_error" {
		t.Errorf("bracket should take priority: got %q, want %q", got, "api_error")
	}
}

func TestExtractSubcategory_noFallbackWithMessage(t *testing.T) {
	// Exit code fallback should NOT trigger when message is present
	got := extractSubcategory("validation_failed", "some unrecognized error", 6)
	if got != "" {
		t.Errorf("should not use exit code fallback when message is present: got %q", got)
	}
}

func TestGenerateFailureID(t *testing.T) {
	d := FailureDetail{
		Ecosystem: "homebrew",
		Timestamp: "2026-02-15T13:45:21Z",
		Package:   "neovim",
	}

	id := generateFailureID(d)
	want := "homebrew-2026-02-15T13-45-21Z-neovim"
	if id != want {
		t.Errorf("generateFailureID() = %q, want %q", id, want)
	}
}

func TestGenerateFailureID_nanoTimestamp(t *testing.T) {
	d := FailureDetail{
		Ecosystem: "homebrew",
		Timestamp: "2026-02-01T18:00:25.957701404Z",
		Package:   "awscli",
	}

	id := generateFailureID(d)
	want := "homebrew-2026-02-01T18-00-25Z-awscli"
	if id != want {
		t.Errorf("generateFailureID() = %q, want %q", id, want)
	}
}

func TestFormatTimestampForID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-02-15T13:45:21Z", "2026-02-15T13-45-21Z"},
		{"2026-02-01T18:00:25.957701404Z", "2026-02-01T18-00-25Z"},
		{"not-a-timestamp", "not-a-timestamp"},
	}

	for _, tt := range tests {
		got := formatTimestampForID(tt.input)
		if got != tt.want {
			t.Errorf("formatTimestampForID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseFailureFilename(t *testing.T) {
	tests := []struct {
		basename  string
		ecosystem string
		batchID   string
	}{
		{"homebrew.jsonl", "homebrew", ""},
		{"homebrew-2026-02-08T02-33-27Z.jsonl", "homebrew", "homebrew-2026-02-08T02-33-27Z"},
		{"failures.jsonl", "failures", ""},
		{"npm-2026-01-15T10-00-00Z.jsonl", "npm", "npm-2026-01-15T10-00-00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.basename, func(t *testing.T) {
			eco, bid := parseFailureFilename(tt.basename)
			if eco != tt.ecosystem {
				t.Errorf("ecosystem: got %q, want %q", eco, tt.ecosystem)
			}
			if bid != tt.batchID {
				t.Errorf("batchID: got %q, want %q", bid, tt.batchID)
			}
		})
	}
}

func TestResolvePackageName(t *testing.T) {
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: "success", Confidence: "curated"},
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	tests := []struct {
		packageID string
		want      string
	}{
		{"homebrew:jq", "jq"},
		{"github:sharkdp/bat", "bat"},
		{"homebrew:unknown", "unknown"}, // Fallback: after colon
		{"github:user/repo", "repo"},    // Fallback: last segment after slash
		{"nocolon", "nocolon"},          // Fallback: whole string
	}

	for _, tt := range tests {
		t.Run(tt.packageID, func(t *testing.T) {
			got := resolvePackageName(tt.packageID, queue)
			if got != tt.want {
				t.Errorf("resolvePackageName(%q) = %q, want %q", tt.packageID, got, tt.want)
			}
		})
	}
}

func TestResolvePackageName_nilQueue(t *testing.T) {
	got := resolvePackageName("homebrew:jq", nil)
	if got != "jq" {
		t.Errorf("resolvePackageName with nil queue: got %q, want %q", got, "jq")
	}
}

func TestExtractEcosystemFromID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"homebrew:jq", "homebrew"},
		{"cargo:ripgrep", "cargo"},
		{"github:sharkdp/bat", "github"},
		{"nocolon", ""},
	}

	for _, tt := range tests {
		got := extractEcosystemFromID(tt.input)
		if got != tt.want {
			t.Errorf("extractEcosystemFromID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoadFailureDetailRecords_legacyFormat(t *testing.T) {
	dir := t.TempDir()
	content := `{"schema_version":1,"ecosystem":"homebrew","environment":"linux-x86_64","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"homebrew:imagemagick","category":"missing_dep","blocked_by":["glib"],"message":"Checking runtime dependencies for imagemagick...\n  Error: recipe glib not found","timestamp":"2026-02-01T00:00:01Z"},{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["dav1d"],"message":"missing dav1d dependency","timestamp":"2026-02-01T00:00:02Z"}]}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 2 {
		t.Fatalf("got %d details, want 2", len(details))
	}

	// Check first record (sorted by timestamp desc, so ffmpeg should be first)
	if details[0].Package != "ffmpeg" {
		t.Errorf("details[0].Package: got %q, want %q", details[0].Package, "ffmpeg")
	}
	if details[0].Ecosystem != "homebrew" {
		t.Errorf("details[0].Ecosystem: got %q, want %q", details[0].Ecosystem, "homebrew")
	}
	if details[0].Category != "missing_dep" {
		t.Errorf("details[0].Category: got %q, want %q", details[0].Category, "missing_dep")
	}
	if details[0].Message != "missing dav1d dependency" {
		t.Errorf("details[0].Message: got %q, want %q", details[0].Message, "missing dav1d dependency")
	}

	// Check IDs are generated
	if details[0].ID == "" {
		t.Error("details[0].ID should not be empty")
	}
	if details[1].ID == "" {
		t.Error("details[1].ID should not be empty")
	}
}

func TestLoadFailureDetailRecords_perRecipeFormat(t *testing.T) {
	dir := t.TempDir()
	content := `{"schema_version":1,"recipe":"procs","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
{"schema_version":1,"recipe":"sd","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew-2026-02-08T02-37-10Z.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 2 {
		t.Fatalf("got %d details, want 2", len(details))
	}

	// Check that ecosystem defaults to homebrew
	for _, d := range details {
		if d.Ecosystem != "homebrew" {
			t.Errorf("Ecosystem: got %q, want %q", d.Ecosystem, "homebrew")
		}
		if d.ExitCode != 6 {
			t.Errorf("ExitCode: got %d, want 6", d.ExitCode)
		}
		// Exit code 6 with no message should give "install_failed" subcategory
		if d.Subcategory != "install_failed" {
			t.Errorf("Subcategory: got %q, want %q", d.Subcategory, "install_failed")
		}
		// BatchID should be derived from filename
		if d.BatchID != "homebrew-2026-02-08T02-37-10Z" {
			t.Errorf("BatchID: got %q, want %q", d.BatchID, "homebrew-2026-02-08T02-37-10Z")
		}
	}
}

func TestLoadFailureDetailRecords_perRecipeDeduplication(t *testing.T) {
	dir := t.TempDir()
	// Same recipe on multiple platforms - should be deduplicated
	content := `{"schema_version":1,"recipe":"procs","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
{"schema_version":1,"recipe":"procs","platform":"linux-alpine-musl-arm64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
{"schema_version":1,"recipe":"procs","platform":"darwin-arm64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew-2026-02-08T02-37-10Z.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	// Should deduplicate to 1 record
	if len(details) != 1 {
		t.Fatalf("got %d details after dedup, want 1", len(details))
	}

	d := details[0]
	if d.Package != "procs" {
		t.Errorf("Package: got %q, want %q", d.Package, "procs")
	}
	if d.Platform != "multiple" {
		t.Errorf("Platform: got %q, want %q", d.Platform, "multiple")
	}
	if len(d.Platforms) != 3 {
		t.Errorf("Platforms: got %d, want 3", len(d.Platforms))
	}
	// Platforms should be sorted
	if d.Platforms[0] != "darwin-arm64" {
		t.Errorf("Platforms[0]: got %q, want %q", d.Platforms[0], "darwin-arm64")
	}
}

func TestLoadFailureDetailRecords_cap(t *testing.T) {
	dir := t.TempDir()
	// Generate more than maxFailureDetails records
	var lines string
	for i := 0; i < 250; i++ {
		lines += `{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"homebrew:pkg` +
			string(rune('a'+i%26)) + `","category":"validation_failed","message":"error","timestamp":"2026-02-01T00:` +
			padInt(i/60) + `:` + padInt(i%60) + `Z"}]}` + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(lines), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) > maxFailureDetails {
		t.Errorf("got %d details, want at most %d", len(details), maxFailureDetails)
	}
}

// padInt formats an int as a zero-padded 2-digit string.
func padInt(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func TestLoadFailureDetailRecords_sortedByTimestampDesc(t *testing.T) {
	dir := t.TempDir()
	content := `{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"homebrew:first","category":"validation_failed","message":"error","timestamp":"2026-02-01T00:00:00Z"}]}
{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-03T00:00:00Z","failures":[{"package_id":"homebrew:third","category":"validation_failed","message":"error","timestamp":"2026-02-03T00:00:00Z"}]}
{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-02T00:00:00Z","failures":[{"package_id":"homebrew:second","category":"validation_failed","message":"error","timestamp":"2026-02-02T00:00:00Z"}]}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(lines(content)), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 3 {
		t.Fatalf("got %d details, want 3", len(details))
	}

	// Should be sorted newest first
	if details[0].Package != "third" {
		t.Errorf("details[0].Package: got %q, want %q (newest first)", details[0].Package, "third")
	}
	if details[1].Package != "second" {
		t.Errorf("details[1].Package: got %q, want %q", details[1].Package, "second")
	}
	if details[2].Package != "first" {
		t.Errorf("details[2].Package: got %q, want %q", details[2].Package, "first")
	}
}

// lines is a no-op helper that returns the input string (used for readability).
func lines(s string) string { return s }

func TestLoadFailureDetailRecords_messageTruncation(t *testing.T) {
	dir := t.TempDir()
	// Create a message longer than maxMessageLength
	longMsg := ""
	for i := 0; i < 600; i++ {
		longMsg += "x"
	}
	content := `{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"homebrew:test","category":"validation_failed","message":"` + longMsg + `","timestamp":"2026-02-01T00:00:00Z"}]}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 1 {
		t.Fatalf("got %d details, want 1", len(details))
	}

	if len(details[0].Message) > maxMessageLength {
		t.Errorf("message length %d exceeds max %d", len(details[0].Message), maxMessageLength)
	}
	if len(details[0].Message) != maxMessageLength {
		t.Errorf("message should be truncated to %d, got %d", maxMessageLength, len(details[0].Message))
	}
}

func TestLoadFailureDetailRecords_emptyDir(t *testing.T) {
	dir := t.TempDir()
	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}
	if details != nil {
		t.Errorf("expected nil for empty dir, got %v", details)
	}
}

func TestLoadFailureDetailRecords_withQueueLookup(t *testing.T) {
	dir := t.TempDir()
	content := `{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"github:sharkdp/bat","category":"validation_failed","message":"test error","timestamp":"2026-02-01T00:00:00Z"}]}
`
	if err := os.WriteFile(filepath.Join(dir, "test.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: "pending", Confidence: "auto"},
		},
	}

	details, err := loadFailureDetailRecords(dir, queue)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 1 {
		t.Fatalf("got %d details, want 1", len(details))
	}

	// Queue lookup should resolve "github:sharkdp/bat" -> "bat"
	if details[0].Package != "bat" {
		t.Errorf("Package: got %q, want %q (resolved via queue)", details[0].Package, "bat")
	}
	if details[0].Ecosystem != "github" {
		t.Errorf("Ecosystem: got %q, want %q", details[0].Ecosystem, "github")
	}
}

func TestLoadFailureDetailRecords_testdata(t *testing.T) {
	// Use the existing testdata directory which has failures.jsonl
	details, err := loadFailureDetailRecords("testdata", nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	// testdata/failures.jsonl has:
	// - 2 legacy records (imagemagick: missing_dep, ffmpeg: missing_dep)
	//   and 1 legacy record (coreutils: validation_failed)
	// - 2 per-recipe records (node: api_error, python: validation_failed)
	if len(details) < 4 {
		t.Errorf("expected at least 4 details from testdata, got %d", len(details))
	}

	// Check that subcategories are extracted where possible
	for _, d := range details {
		if d.ID == "" {
			t.Errorf("ID should not be empty for %s", d.Package)
		}
		if d.Ecosystem == "" {
			t.Errorf("Ecosystem should not be empty for %s", d.Package)
		}
	}
}

func TestLoadFailureDetailRecords_mixedFormats(t *testing.T) {
	dir := t.TempDir()
	// File with both legacy and per-recipe records (like real homebrew.jsonl)
	content := `{"schema_version":1,"ecosystem":"homebrew","environment":"linux-x86_64","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"homebrew:python@3.14","category":"deterministic_insufficient","message":"deterministic generation failed: [api_error] failed to fetch bottle data for formula python@3.14","timestamp":"2026-02-02T01:53:32Z"}]}
{"schema_version":1,"recipe":"procs","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 2 {
		t.Fatalf("got %d details, want 2", len(details))
	}

	// Find the legacy record
	var legacy, perRecipe *FailureDetail
	for i := range details {
		if details[i].Package == "python@3.14" {
			legacy = &details[i]
		}
		if details[i].Package == "procs" {
			perRecipe = &details[i]
		}
	}

	if legacy == nil {
		t.Fatal("expected legacy format record for python@3.14")
	}
	// Bracketed tag should be extracted
	if legacy.Subcategory != "api_error" {
		t.Errorf("legacy Subcategory: got %q, want %q", legacy.Subcategory, "api_error")
	}
	if legacy.Message == "" {
		t.Error("legacy Message should not be empty")
	}

	if perRecipe == nil {
		t.Fatal("expected per-recipe format record for procs")
	}
	if perRecipe.ExitCode != 6 {
		t.Errorf("perRecipe ExitCode: got %d, want 6", perRecipe.ExitCode)
	}
	if perRecipe.Subcategory != "install_failed" {
		t.Errorf("perRecipe Subcategory: got %q, want %q", perRecipe.Subcategory, "install_failed")
	}
}

func TestDeduplicateFailureDetails_noDedup(t *testing.T) {
	// Legacy records (with messages) should not be deduplicated
	details := []FailureDetail{
		{Package: "pkg1", BatchID: "batch-1", Message: "error1", Timestamp: "2026-02-01T00:00:00Z"},
		{Package: "pkg1", BatchID: "batch-1", Message: "error2", Timestamp: "2026-02-01T00:00:01Z"},
	}

	result := deduplicateFailureDetails(details)
	if len(result) != 2 {
		t.Errorf("legacy records should not be deduplicated: got %d, want 2", len(result))
	}
}

func TestDeduplicateFailureDetails_differentPackages(t *testing.T) {
	// Different packages in same batch should not be grouped
	details := []FailureDetail{
		{Package: "procs", BatchID: "batch-1", Platform: "linux-x86_64", ExitCode: 6, Timestamp: "2026-02-01T00:00:00Z"},
		{Package: "sd", BatchID: "batch-1", Platform: "linux-x86_64", ExitCode: 6, Timestamp: "2026-02-01T00:00:00Z"},
	}

	result := deduplicateFailureDetails(details)
	if len(result) != 2 {
		t.Errorf("different packages should not be grouped: got %d, want 2", len(result))
	}
}

func TestDeduplicateFailureDetails_singlePlatform(t *testing.T) {
	// Single platform record should keep its original Platform field
	details := []FailureDetail{
		{Package: "procs", BatchID: "batch-1", Platform: "linux-x86_64", ExitCode: 6, Timestamp: "2026-02-01T00:00:00Z"},
	}

	result := deduplicateFailureDetails(details)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	if result[0].Platform != "linux-x86_64" {
		t.Errorf("Platform: got %q, want %q", result[0].Platform, "linux-x86_64")
	}
	if result[0].Platforms != nil {
		t.Errorf("Platforms should be nil for single platform: %v", result[0].Platforms)
	}
}

func TestLoadFailureDetailRecords_malformedLines(t *testing.T) {
	dir := t.TempDir()
	content := `not valid json
{"schema_version":1,"recipe":"procs","platform":"linux-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-08T02:37:10Z"}
also not json {"broken
`
	if err := os.WriteFile(filepath.Join(dir, "mixed.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	// Should skip malformed lines and process valid ones
	if len(details) != 1 {
		t.Errorf("got %d details, want 1 (skipping malformed)", len(details))
	}
}

func TestLoadFailureDetailRecords_subcategoriesExtracted(t *testing.T) {
	dir := t.TempDir()
	content := `{"schema_version":1,"ecosystem":"homebrew","updated_at":"2026-02-01T00:00:00Z","failures":[{"package_id":"homebrew:pkgconf","category":"validation_failed","message":"Error: recipe already exists at recipes/p/pkgconf.toml\nUse --force to overwrite","timestamp":"2026-02-01T00:00:00Z"},{"package_id":"homebrew:python@3.14","category":"deterministic_insufficient","message":"deterministic generation failed: [api_error] failed to fetch bottle data","timestamp":"2026-02-01T00:00:01Z"},{"package_id":"homebrew:node","category":"missing_dep","message":"missing dependencies","timestamp":"2026-02-01T00:00:02Z"}]}
`
	if err := os.WriteFile(filepath.Join(dir, "test.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := loadFailureDetailRecords(dir, nil)
	if err != nil {
		t.Fatalf("loadFailureDetailRecords: %v", err)
	}

	if len(details) != 3 {
		t.Fatalf("got %d details, want 3", len(details))
	}

	// Build a map by package for easier assertion
	byPkg := make(map[string]FailureDetail)
	for _, d := range details {
		byPkg[d.Package] = d
	}

	// pkgconf: "already exists" -> recipe_already_exists
	if byPkg["pkgconf"].Subcategory != "recipe_already_exists" {
		t.Errorf("pkgconf Subcategory: got %q, want %q", byPkg["pkgconf"].Subcategory, "recipe_already_exists")
	}

	// python@3.14: [api_error] -> api_error
	if byPkg["python@3.14"].Subcategory != "api_error" {
		t.Errorf("python@3.14 Subcategory: got %q, want %q", byPkg["python@3.14"].Subcategory, "api_error")
	}

	// node: "missing dependencies" -> no subcategory match
	if byPkg["node"].Subcategory != "" {
		t.Errorf("node Subcategory: got %q, want %q (no match)", byPkg["node"].Subcategory, "")
	}
}
