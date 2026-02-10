package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PlanCacheKey uniquely identifies a cached plan.
// The key is based on the OUTPUT of version resolution (the resolved version),
// not the user's input. This means "ripgrep" and "ripgrep@14.1.0" that resolve
// to the same version will share the same cache key.
//
// ContentHash is computed from the plan's functional content (excluding timestamps
// and provenance) and is used to validate that a cached plan matches freshly
// generated content. It's populated after plan generation or when loading from cache.
type PlanCacheKey struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`                // RESOLVED version (e.g., "14.1.0")
	Platform    string `json:"platform"`               // e.g., "linux-amd64"
	ContentHash string `json:"content_hash,omitempty"` // SHA256 of normalized plan content
}

// CacheKeyFor generates a cache key from version resolution output.
// This should be called AFTER version resolution completes.
// Note: ContentHash is not set by this function; call ComputePlanContentHash
// separately after plan generation to populate it.
func CacheKeyFor(tool, resolvedVersion, os, arch string) PlanCacheKey {
	return PlanCacheKey{
		Tool:     tool,
		Version:  resolvedVersion,
		Platform: fmt.Sprintf("%s-%s", os, arch),
	}
}

// CacheKeyWithHash generates a cache key with content hash.
// Use this when you have a plan and want to create a key for cache validation.
func CacheKeyWithHash(tool, resolvedVersion, os, arch string, plan *InstallationPlan) PlanCacheKey {
	key := CacheKeyFor(tool, resolvedVersion, os, arch)
	if plan != nil {
		key.ContentHash = ComputePlanContentHash(plan)
	}
	return key
}

// ValidateCachedPlan checks if a cached plan is still valid for the given cache key.
// Validation checks:
//   - Format version matches current PlanFormatVersion
//   - Platform (OS-Arch) matches the key
//   - Content hash matches (if ContentHash is set in the key)
//
// Returns nil if valid, or an error describing why the plan is invalid.
func ValidateCachedPlan(plan *InstallationPlan, key PlanCacheKey) error {
	// Check format version
	if plan.FormatVersion != PlanFormatVersion {
		return fmt.Errorf("plan format version %d is outdated (current: %d)",
			plan.FormatVersion, PlanFormatVersion)
	}

	// Parse platform from key (format: "os-arch")
	keyOS, keyArch, found := strings.Cut(key.Platform, "-")
	if !found {
		return fmt.Errorf("invalid platform format in cache key: %q (expected \"os-arch\")", key.Platform)
	}

	// Check platform
	if plan.Platform.OS != keyOS || plan.Platform.Arch != keyArch {
		return fmt.Errorf("plan platform %s-%s does not match %s",
			plan.Platform.OS, plan.Platform.Arch, key.Platform)
	}

	// Check content hash if provided
	if key.ContentHash != "" {
		cachedHash := ComputePlanContentHash(plan)
		if cachedHash != key.ContentHash {
			return fmt.Errorf("plan content hash mismatch: cached %s, expected %s",
				cachedHash[:12]+"...", key.ContentHash[:12]+"...")
		}
	}

	return nil
}

// ComputePlanContentHash computes a deterministic SHA256 hash of the plan's
// functional content. The hash excludes non-functional fields (GeneratedAt,
// RecipeSource) to ensure that plans with identical steps produce identical
// hashes regardless of when or how they were generated.
//
// This enables content-based cache validation: different recipes that produce
// identical installation plans can share cached artifacts.
func ComputePlanContentHash(plan *InstallationPlan) string {
	normalized := planContentForHashing(plan)
	data, _ := json.Marshal(normalized)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// planForHashing is a normalized representation of InstallationPlan for hashing.
// Fields are in alphabetical order to ensure deterministic JSON output.
// Non-functional fields (GeneratedAt, RecipeSource) are excluded.
type planForHashing struct {
	Dependencies  []depForHashing  `json:"dependencies,omitempty"`
	Deterministic bool             `json:"deterministic"`
	FormatVersion int              `json:"format_version"`
	Platform      platformForHash  `json:"platform"`
	RecipeType    string           `json:"recipe_type,omitempty"`
	Steps         []stepForHashing `json:"steps"`
	Tool          string           `json:"tool"`
	Verify        *verifyForHash   `json:"verify,omitempty"`
	Version       string           `json:"version"`
}

// platformForHash is a normalized Platform for hashing.
type platformForHash struct {
	Arch        string `json:"arch"`
	LinuxFamily string `json:"linux_family,omitempty"`
	OS          string `json:"os"`
}

// stepForHashing is a normalized ResolvedStep for hashing.
type stepForHashing struct {
	Action        string      `json:"action"`
	Checksum      string      `json:"checksum,omitempty"`
	Deterministic bool        `json:"deterministic"`
	Evaluable     bool        `json:"evaluable"`
	Params        interface{} `json:"params"` // Uses sortedParams for deterministic ordering
	Size          int64       `json:"size,omitempty"`
	URL           string      `json:"url,omitempty"`
}

// depForHashing is a normalized DependencyPlan for hashing.
type depForHashing struct {
	Dependencies []depForHashing  `json:"dependencies,omitempty"`
	RecipeType   string           `json:"recipe_type,omitempty"`
	Steps        []stepForHashing `json:"steps"`
	Tool         string           `json:"tool"`
	Verify       *verifyForHash   `json:"verify,omitempty"`
	Version      string           `json:"version"`
}

// verifyForHash is a normalized PlanVerify for hashing.
type verifyForHash struct {
	Command string `json:"command,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

// planContentForHashing converts an InstallationPlan to a normalized
// representation suitable for deterministic hashing. It excludes GeneratedAt
// and RecipeSource, and sorts map keys for consistent output.
func planContentForHashing(plan *InstallationPlan) planForHashing {
	result := planForHashing{
		Deterministic: plan.Deterministic,
		FormatVersion: plan.FormatVersion,
		Platform: platformForHash{
			Arch:        plan.Platform.Arch,
			LinuxFamily: plan.Platform.LinuxFamily,
			OS:          plan.Platform.OS,
		},
		RecipeType: plan.RecipeType,
		Tool:       plan.Tool,
		Version:    plan.Version,
	}

	// Convert steps
	result.Steps = make([]stepForHashing, len(plan.Steps))
	for i, step := range plan.Steps {
		result.Steps[i] = stepForHashing{
			Action:        step.Action,
			Checksum:      step.Checksum,
			Deterministic: step.Deterministic,
			Evaluable:     step.Evaluable,
			Params:        sortedParams(step.Params),
			Size:          step.Size,
			URL:           step.URL,
		}
	}

	// Convert dependencies recursively
	result.Dependencies = convertDepsForHashing(plan.Dependencies)

	// Convert verify if present
	if plan.Verify != nil {
		result.Verify = &verifyForHash{
			Command: plan.Verify.Command,
			Pattern: plan.Verify.Pattern,
		}
	}

	return result
}

// convertDepsForHashing recursively converts DependencyPlan slices.
func convertDepsForHashing(deps []DependencyPlan) []depForHashing {
	if len(deps) == 0 {
		return nil
	}
	result := make([]depForHashing, len(deps))
	for i, dep := range deps {
		result[i] = depForHashing{
			Dependencies: convertDepsForHashing(dep.Dependencies),
			RecipeType:   dep.RecipeType,
			Tool:         dep.Tool,
			Version:      dep.Version,
		}

		// Convert steps
		result[i].Steps = make([]stepForHashing, len(dep.Steps))
		for j, step := range dep.Steps {
			result[i].Steps[j] = stepForHashing{
				Action:        step.Action,
				Checksum:      step.Checksum,
				Deterministic: step.Deterministic,
				Evaluable:     step.Evaluable,
				Params:        sortedParams(step.Params),
				Size:          step.Size,
				URL:           step.URL,
			}
		}

		// Convert verify if present
		if dep.Verify != nil {
			result[i].Verify = &verifyForHash{
				Command: dep.Verify.Command,
				Pattern: dep.Verify.Pattern,
			}
		}
	}
	return result
}

// sortedParams converts a map[string]interface{} to a deterministically-ordered
// representation. Go's json.Marshal sorts map keys alphabetically (since Go 1.12),
// but we explicitly sort to be clear about the contract and handle nested structures.
func sortedParams(params map[string]interface{}) interface{} {
	if params == nil {
		return nil
	}
	if len(params) == 0 {
		return map[string]interface{}{}
	}

	// Get sorted keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build ordered map (Go's json.Marshal will maintain key order for maps)
	result := make(map[string]interface{}, len(params))
	for _, k := range keys {
		result[k] = sortValue(params[k])
	}
	return result
}

// sortValue recursively processes values that may contain nested maps or slices.
func sortValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return sortedParams(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = sortValue(item)
		}
		return result
	default:
		return v
	}
}

// ChecksumMismatchError indicates that a downloaded asset's checksum doesn't match
// the expected checksum recorded in the installation plan. This could indicate:
//   - A legitimate release update (re-tagged release)
//   - A supply chain attack (malicious modification)
type ChecksumMismatchError struct {
	Tool             string // Tool being installed
	Version          string // Version being installed
	URL              string // URL of the mismatched download
	ExpectedChecksum string // Checksum from the installation plan
	ActualChecksum   string // Checksum of the downloaded file
}

// Error implements the error interface with a user-friendly message
// that explains the issue and provides a recovery path.
func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf(`checksum mismatch for %s

Expected: %s
Got:      %s

The upstream asset has changed since the installation plan was generated.
This could indicate:
- A legitimate release update (re-tagged release)
- A supply chain attack (malicious modification)

To proceed with the new asset, regenerate the plan:
    tsuku install %s@%s --fresh

To investigate, compare with upstream release notes or checksums.`,
		e.URL, e.ExpectedChecksum, e.ActualChecksum, e.Tool, e.Version)
}
