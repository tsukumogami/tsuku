package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// FailureDetail represents an individual failure record for the dashboard.
// It normalizes data from both legacy batch and per-recipe JSONL formats
// into a single structure suitable for failure list and detail pages.
type FailureDetail struct {
	ID          string   `json:"id"` // "<ecosystem>-<timestamp>-<package>"
	Package     string   `json:"package"`
	Ecosystem   string   `json:"ecosystem"`
	Category    string   `json:"category"`
	Subcategory string   `json:"subcategory,omitempty"`
	Message     string   `json:"message,omitempty"`   // legacy format only, up to 500 chars
	ExitCode    int      `json:"exit_code,omitempty"` // per-recipe format
	Platform    string   `json:"platform,omitempty"`
	Platforms   []string `json:"platforms,omitempty"` // for multi-platform dedup
	BatchID     string   `json:"batch_id,omitempty"`
	Timestamp   string   `json:"timestamp"`
	WorkflowURL string   `json:"workflow_url,omitempty"`
}

// maxFailureDetails is the cap on failure_details entries in dashboard.json.
const maxFailureDetails = 200

// maxMessageLength is the maximum length for error messages stored in FailureDetail.
const maxMessageLength = 500

// knownSubcategories validates bracket-extracted tags against an allowlist.
var knownSubcategories = map[string]bool{
	"api_error":       true,
	"no_bottles":      true,
	"complex_archive": true,
	"no_binaries":     true,
	"verify_failed":   true,
	"install_failed":  true,
	"timeout":         true,
	"rate_limited":    true,
}

// categoryRemap maps old category strings to the canonical pipeline taxonomy.
// Categories not in this map pass through unchanged.
var categoryRemap = map[string]string{
	"api_error":                  "network_error",
	"validation_failed":          "install_failed",
	"deterministic_insufficient": "generation_failed",
	"deterministic":              "generation_failed",
	"timeout":                    "network_error",
	"network":                    "network_error",
}

// remapCategory translates old category strings to canonical names.
// Categories already in the canonical taxonomy pass through unchanged.
func remapCategory(category string) string {
	if canonical, ok := categoryRemap[category]; ok {
		return canonical
	}
	return category
}

// extractSubcategory classifies a failure into a subcategory using a 3-level
// strategy: bracketed tags (highest confidence), regex pattern matching, and
// exit code fallback (lowest confidence). Returns empty string if no
// subcategory can be determined.
func extractSubcategory(category, message string, exitCode int) string {
	// Level 1: Bracketed tags (highest confidence)
	// Matches patterns like "deterministic generation failed: [no_bottles] ..."
	if idx := strings.Index(message, "["); idx >= 0 {
		if end := strings.Index(message[idx:], "]"); end > 1 {
			tag := message[idx+1 : idx+end]
			if knownSubcategories[tag] {
				return tag
			}
		}
	}

	// Level 2: Regex pattern matching
	msg := strings.ToLower(message)
	switch {
	case strings.Contains(msg, "no bottle") || strings.Contains(msg, "bottle not found"):
		return "no_bottle"
	case strings.Contains(msg, "no executables") || strings.Contains(msg, "no binaries"):
		return "binary_discovery_failed"
	case strings.Contains(msg, "version pattern") || strings.Contains(msg, "failed to verify") || strings.Contains(msg, "verification"):
		return "verify_pattern_mismatch"
	case strings.Contains(msg, "already exists") || strings.Contains(msg, "use --force"):
		return "recipe_already_exists"
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429"):
		return "rate_limited"
	case strings.Contains(msg, "5xx") || strings.Contains(msg, "unavailable"):
		return "upstream_unavailable"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout"
	}

	// Level 3: Exit code fallback (lowest confidence)
	if message == "" {
		switch exitCode {
		case 6:
			return "install_failed"
		case 7:
			return "verify_failed"
		case 9:
			return "deterministic_failed"
		}
	}

	return ""
}

// loadFailureDetailRecords reads all JSONL files in a directory and produces
// individual FailureDetail records. It normalizes both legacy batch format and
// per-recipe format, deduplicates per-recipe records by (package, batch_id),
// extracts subcategories, sorts by timestamp descending, and caps at
// maxFailureDetails.
//
// The queue parameter is used for resolving package names from legacy format
// package IDs (e.g., "homebrew:jq" -> "jq"). It may be nil.
func loadFailureDetailRecords(dir string, queue *batch.UnifiedQueue) ([]FailureDetail, error) {
	pattern := filepath.Join(dir, "*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob failures: %w", err)
	}

	if len(files) == 0 {
		return nil, nil
	}

	var allDetails []FailureDetail

	for _, path := range files {
		details, err := loadFailureDetailsFromFile(path, queue)
		if err != nil {
			continue // Skip files that can't be read
		}
		allDetails = append(allDetails, details...)
	}

	// Deduplicate per-recipe records by (package, batch_id)
	allDetails = deduplicateFailureDetails(allDetails)

	// Extract subcategories only for records that don't already have one.
	// Records with a structured subcategory from JSONL retain that value.
	for i := range allDetails {
		if allDetails[i].Subcategory == "" {
			allDetails[i].Subcategory = extractSubcategory(
				allDetails[i].Category,
				allDetails[i].Message,
				allDetails[i].ExitCode,
			)
		}
	}

	// Generate IDs
	for i := range allDetails {
		allDetails[i].ID = generateFailureID(allDetails[i])
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(allDetails, func(i, j int) bool {
		return allDetails[i].Timestamp > allDetails[j].Timestamp
	})

	// Cap at maxFailureDetails
	if len(allDetails) > maxFailureDetails {
		allDetails = allDetails[:maxFailureDetails]
	}

	return allDetails, nil
}

// loadFailureDetailsFromFile reads a single JSONL file and returns FailureDetail records.
func loadFailureDetailsFromFile(path string, queue *batch.UnifiedQueue) ([]FailureDetail, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Extract ecosystem and batch ID hint from filename.
	// Filenames like "homebrew-2026-02-08T02-33-27Z.jsonl" or "homebrew.jsonl".
	baseName := filepath.Base(path)
	filenameEcosystem, filenameBatchID := parseFailureFilename(baseName)

	var details []FailureDetail
	scanner := bufio.NewScanner(file)
	// Increase buffer size for large legacy format lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record FailureRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue // Skip malformed lines
		}

		// Handle legacy batch format with failures array
		if len(record.Failures) > 0 {
			eco := record.Ecosystem
			if eco == "" {
				eco = filenameEcosystem
			}
			batchID := filenameBatchID

			for _, f := range record.Failures {
				pkg := resolvePackageName(f.PackageID, queue)
				pkgEco := extractEcosystemFromID(f.PackageID)
				if pkgEco == "" {
					pkgEco = eco
				}

				msg := f.Message
				if len(msg) > maxMessageLength {
					msg = msg[:maxMessageLength]
				}

				details = append(details, FailureDetail{
					Package:     pkg,
					Ecosystem:   pkgEco,
					Category:    remapCategory(f.Category),
					Subcategory: f.Subcategory,
					Message:     msg,
					BatchID:     batchID,
					Timestamp:   f.Timestamp,
				})
			}
		}

		// Handle per-recipe format
		if record.Recipe != "" && record.Category != "" {
			eco := record.Ecosystem
			if eco == "" {
				eco = filenameEcosystem
			}
			if eco == "" {
				eco = "unknown" // Default when ecosystem cannot be determined
			}

			ts := record.Timestamp
			if ts == "" {
				ts = record.UpdatedAt
			}

			details = append(details, FailureDetail{
				Package:     record.Recipe,
				Ecosystem:   eco,
				Category:    remapCategory(record.Category),
				Subcategory: record.Subcategory,
				ExitCode:    record.ExitCode,
				Platform:    record.Platform,
				BatchID:     filenameBatchID,
				Timestamp:   ts,
			})
		}
	}

	return details, scanner.Err()
}

// FailureRecord.Timestamp field - we need to add it to the existing struct.
// The per-recipe format includes a "timestamp" field at the record level.

// deduplicateFailureDetails groups per-recipe records by (package, batch_id)
// into single records with Platform="multiple" and a Platforms list. Legacy
// records (those with a Message field) are not deduplicated.
func deduplicateFailureDetails(details []FailureDetail) []FailureDetail {
	type dedupKey struct {
		Package string
		BatchID string
	}

	var result []FailureDetail
	groups := make(map[dedupKey][]FailureDetail)

	for _, d := range details {
		// Only deduplicate per-recipe records (those with a Platform but no Message)
		if d.Platform != "" && d.Message == "" && d.BatchID != "" {
			key := dedupKey{Package: d.Package, BatchID: d.BatchID}
			groups[key] = append(groups[key], d)
		} else {
			result = append(result, d)
		}
	}

	// Merge grouped records
	for _, group := range groups {
		if len(group) == 1 {
			result = append(result, group[0])
			continue
		}

		// Merge multiple platform records into one
		merged := group[0]
		platforms := make([]string, 0, len(group))
		for _, d := range group {
			platforms = append(platforms, d.Platform)
		}
		sort.Strings(platforms)

		merged.Platform = "multiple"
		merged.Platforms = platforms
		result = append(result, merged)
	}

	return result
}

// generateFailureID creates a URL-safe ID for a failure detail record.
// Format: "<ecosystem>-<timestamp>-<package>" where timestamp uses dashes
// instead of colons for URL safety.
func generateFailureID(d FailureDetail) string {
	ts := formatTimestampForID(d.Timestamp)
	return d.Ecosystem + "-" + ts + "-" + d.Package
}

// formatTimestampForID converts an RFC3339 timestamp to a URL-safe format
// by replacing colons with dashes.
func formatTimestampForID(ts string) string {
	// Parse and reformat for consistency
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try RFC3339Nano
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			// Fall back to replacing colons directly
			return strings.ReplaceAll(ts, ":", "-")
		}
	}
	return t.UTC().Format("2006-01-02T15-04-05Z")
}

// resolvePackageName looks up a package name from its ID using the unified queue.
// Falls back to extracting the name from the ID string.
func resolvePackageName(packageID string, queue *batch.UnifiedQueue) string {
	if queue != nil {
		for _, entry := range queue.Entries {
			if entry.Source == packageID {
				return entry.Name
			}
		}
	}
	// Fallback: extract the part after the first colon
	if idx := strings.Index(packageID, ":"); idx >= 0 {
		name := packageID[idx+1:]
		// For github-style sources like "github:user/repo", take the last segment
		if slashIdx := strings.LastIndex(name, "/"); slashIdx >= 0 {
			return name[slashIdx+1:]
		}
		return name
	}
	return packageID
}

// extractEcosystemFromID extracts the ecosystem prefix from a package ID.
// For example, "homebrew:jq" returns "homebrew".
func extractEcosystemFromID(packageID string) string {
	if idx := strings.Index(packageID, ":"); idx >= 0 {
		return packageID[:idx]
	}
	return ""
}

// parseFailureFilename extracts ecosystem and batch ID from a failure filename.
// Filenames follow the pattern "<ecosystem>-<timestamp>.jsonl" or "<ecosystem>.jsonl".
func parseFailureFilename(basename string) (ecosystem, batchID string) {
	// Strip .jsonl extension
	name := strings.TrimSuffix(basename, ".jsonl")

	// Try to find a timestamp in the filename
	// Format: "homebrew-2026-02-08T02-33-27Z"
	// The ecosystem is everything before the first dash that's followed by a 4-digit year
	parts := strings.SplitN(name, "-", 2)
	if len(parts) == 1 {
		// Simple filename like "homebrew.jsonl"
		return name, ""
	}

	ecosystem = parts[0]
	rest := parts[1]

	// Check if the rest starts with a year-like pattern (4 digits)
	if len(rest) >= 4 && rest[0] >= '0' && rest[0] <= '9' && rest[1] >= '0' && rest[1] <= '9' && rest[2] >= '0' && rest[2] <= '9' && rest[3] >= '0' && rest[3] <= '9' {
		batchID = ecosystem + "-" + rest
	}

	return ecosystem, batchID
}
