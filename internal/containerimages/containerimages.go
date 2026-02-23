// Package containerimages provides a centralized mapping of Linux families to
// container images and infrastructure packages. The mapping is defined in
// container-images.json at the repo root and embedded into the binary at build
// time via go:embed.
//
// A go:generate directive copies the root JSON into this package directory so
// the embed directive can find it. Run `go generate ./internal/containerimages/...`
// (or `make build`) to refresh the local copy before building.
package containerimages

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:generate cp ../../container-images.json container-images.json

//go:embed container-images.json
var rawJSON []byte

// familyConfig holds the container image and infrastructure packages for a
// Linux family.
type familyConfig struct {
	Image         string              `json:"image"`
	InfraPackages map[string][]string `json:"infra_packages"`
}

// configs holds the full parsed configuration, populated at init time.
var configs map[string]familyConfig

// images holds the family-to-image projection, populated at init time.
// Kept for backward compatibility with existing callers.
var images map[string]string

func init() {
	if err := json.Unmarshal(rawJSON, &configs); err != nil {
		panic(fmt.Sprintf("containerimages: invalid embedded container-images.json: %v", err))
	}
	if _, ok := configs["debian"]; !ok {
		panic("containerimages: embedded container-images.json missing required \"debian\" entry")
	}

	// Validate that every entry has the three required infra_packages categories.
	required := []string{"core", "network", "build"}
	for family, cfg := range configs {
		for _, cat := range required {
			if _, ok := cfg.InfraPackages[cat]; !ok {
				panic(fmt.Sprintf("containerimages: family %q missing infra_packages.%s", family, cat))
			}
		}
	}

	// Build the image-only projection for backward compat.
	images = make(map[string]string, len(configs))
	for family, cfg := range configs {
		images[family] = cfg.Image
	}
}

// ImageForFamily returns the container image for a Linux family (e.g. "debian",
// "alpine"). Returns ("", false) if the family is not in the config.
func ImageForFamily(family string) (string, bool) {
	img, ok := images[family]
	return img, ok
}

// DefaultImage returns the container image for the "debian" family, which is
// the default used for simple binary installations. It panics if the embedded
// JSON is missing the "debian" entry, but the init function validates this so a
// panic here means the binary was built with a corrupt embed.
func DefaultImage() string {
	img, ok := images["debian"]
	if !ok {
		panic("containerimages: embedded container-images.json missing required \"debian\" entry")
	}
	return img
}

// Families returns a sorted list of all known Linux family names.
func Families() []string {
	fams := make([]string, 0, len(images))
	for f := range images {
		fams = append(fams, f)
	}
	sort.Strings(fams)
	return fams
}

// InfraPackages returns the infrastructure package list for a Linux family and
// category. Category is one of "core", "network", or "build". Returns nil if
// the family is unknown or the category has no packages.
func InfraPackages(family, category string) []string {
	cfg, ok := configs[family]
	if !ok {
		return nil
	}
	pkgs := cfg.InfraPackages[category]
	if len(pkgs) == 0 {
		return nil
	}
	// Return a copy to prevent callers from mutating the embedded data.
	out := make([]string, len(pkgs))
	copy(out, pkgs)
	return out
}
