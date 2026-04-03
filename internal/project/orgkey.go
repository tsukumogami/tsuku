package project

import (
	"fmt"
	"strings"
)

// SplitOrgKey splits an org-scoped tool key into its distributed source and
// bare recipe name. It handles the formats used in .tsuku.toml keys and CLI args:
//
//   - "tsukumogami/koto"          -> ("tsukumogami/koto", "koto", true, nil)
//   - "tsukumogami/registry:tool" -> ("tsukumogami/registry", "tool", true, nil)
//   - "node"                      -> ("", "node", false, nil)
//
// Returns an error for path traversal attempts or malformed source strings.
func SplitOrgKey(key string) (source, bare string, isOrgScoped bool, err error) {
	if !strings.Contains(key, "/") {
		return "", key, false, nil
	}

	// Reject path traversal
	if strings.Contains(key, "..") {
		return "", "", false, fmt.Errorf("invalid tool key %q: path traversal not allowed", key)
	}

	// Strip version suffix (@version) if present
	nameWithoutVersion := key
	if atIdx := strings.LastIndex(key, "@"); atIdx > 0 {
		nameWithoutVersion = key[:atIdx]
	}

	// Split source from recipe name (: separator)
	source = nameWithoutVersion
	recipeName := ""
	if colonIdx := strings.Index(nameWithoutVersion, ":"); colonIdx > 0 && colonIdx < len(nameWithoutVersion)-1 {
		source = nameWithoutVersion[:colonIdx]
		recipeName = nameWithoutVersion[colonIdx+1:]
	}

	// Validate source format: must be exactly "owner/repo"
	parts := strings.SplitN(source, "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false, fmt.Errorf("invalid tool key %q: source must be owner/repo format", key)
	}

	// Default recipe name to repo name
	if recipeName == "" {
		recipeName = parts[1]
	}

	return source, recipeName, true, nil
}
