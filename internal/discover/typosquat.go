package discover

import "strings"

// TyposquatWarning indicates a requested tool name is suspiciously similar
// to a known registry entry. The check catches common typosquatting attempts
// where attackers register packages with names close to popular tools.
type TyposquatWarning struct {
	Requested string // The tool name the user requested
	Similar   string // The registry entry with similar name
	Distance  int    // Levenshtein edit distance (1 or 2)
	Source    string // The source of the similar tool (e.g., "crates.io:bat")
}

// CheckTyposquat compares a tool name against registry entries.
// Returns a warning if the name is suspiciously similar (distance 1-2).
// Exact matches (distance 0) do not trigger warnings - if the tool exists
// in the registry, there's nothing suspicious.
// Returns nil when no similar names are found.
func CheckTyposquat(toolName string, registry *DiscoveryRegistry) *TyposquatWarning {
	if registry == nil {
		return nil
	}

	normalized := strings.ToLower(toolName)

	// If the tool exists exactly in the registry, no warning needed
	if _, exists := registry.Lookup(toolName); exists {
		return nil
	}

	// Find the closest match within the threshold
	var bestMatch *TyposquatWarning
	bestDist := 3 // Start above threshold

	for name, entry := range registry.Tools {
		dist := levenshtein(normalized, strings.ToLower(name))
		if dist > 0 && dist <= 2 && dist < bestDist {
			bestDist = dist
			bestMatch = &TyposquatWarning{
				Requested: toolName,
				Similar:   name,
				Distance:  dist,
				Source:    entry.Builder + ":" + entry.Source,
			}
			// Early exit on distance 1 (can't get better)
			if dist == 1 {
				return bestMatch
			}
		}
	}
	return bestMatch
}

// levenshtein computes the Levenshtein edit distance between two strings.
// The distance is the minimum number of single-character edits (insertions,
// deletions, or substitutions) required to transform one string into another.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use []rune for proper Unicode handling
	ra := []rune(a)
	rb := []rune(b)

	// Create a single row for space optimization (O(min(m,n)) space)
	if len(ra) > len(rb) {
		ra, rb = rb, ra
	}

	prev := make([]int, len(ra)+1)
	curr := make([]int, len(ra)+1)

	// Initialize first row
	for i := range prev {
		prev[i] = i
	}

	for j := 1; j <= len(rb); j++ {
		curr[0] = j
		for i := 1; i <= len(ra); i++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			curr[i] = min(
				prev[i]+1,      // deletion
				curr[i-1]+1,    // insertion
				prev[i-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[len(ra)]
}
