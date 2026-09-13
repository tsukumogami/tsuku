package project

import (
	"fmt"
	"os"
)

// parseConfigFile reads a path and decodes it.
//
// It lives in the test files because production no longer decodes from a path:
// the walk opens the object it decided about and decodes that handle's bytes,
// and a path-based reader in the package would be an unused second way in.
// The decoding tests are about what the decoder makes of some bytes, not about
// which object they came from, so they keep the shape they were written in.
func parseConfigFile(path string) (*ProjectConfig, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading: %w", err)
	}
	return decodeConfig(data)
}
