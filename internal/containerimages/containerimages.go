// Package containerimages provides a centralized mapping of Linux families to
// container images. The mapping is defined in container-images.json at the repo
// root and embedded into the binary at build time via go:embed.
//
// A go:generate directive copies the root JSON into this package directory so
// the embed directive can find it. Run `go generate ./internal/containerimages/...`
// (or `make build`) to refresh the local copy before building.
package containerimages

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:generate cp ../../container-images.json container-images.json

//go:embed container-images.json
var rawJSON []byte

// images holds the parsed family-to-image mapping, populated at init time.
var images map[string]string

func init() {
	if err := json.Unmarshal(rawJSON, &images); err != nil {
		panic(fmt.Sprintf("containerimages: invalid embedded container-images.json: %v", err))
	}
	if _, ok := images["debian"]; !ok {
		panic("containerimages: embedded container-images.json missing required \"debian\" entry")
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
	return fams
}
