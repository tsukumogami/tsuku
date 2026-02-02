package discover

import (
	"fmt"

	"github.com/tsukumogami/tsuku/internal/builders"
)

// QualityFilter rejects low-quality ecosystem probe matches using per-registry
// thresholds. A match passes if ANY threshold is met (OR logic). Unknown
// builders are accepted by default (fail-open).
type QualityFilter struct {
	thresholds map[string]qualityThreshold
}

type qualityThreshold struct {
	MinDownloads    int
	MinVersionCount int
}

// NewQualityFilter creates a filter with default per-registry thresholds.
func NewQualityFilter() *QualityFilter {
	return &QualityFilter{
		thresholds: map[string]qualityThreshold{
			"crates.io": {MinDownloads: 100, MinVersionCount: 5},
		},
	}
}

// Accept checks whether the probe result meets quality thresholds for its builder.
// Returns true if the result should be kept, along with a reason string.
func (f *QualityFilter) Accept(builderName string, result *builders.ProbeResult) (bool, string) {
	thresh, ok := f.thresholds[builderName]
	if !ok {
		return true, "no threshold configured"
	}

	if result.Downloads >= thresh.MinDownloads {
		return true, fmt.Sprintf("downloads %d >= %d", result.Downloads, thresh.MinDownloads)
	}
	if result.VersionCount >= thresh.MinVersionCount {
		return true, fmt.Sprintf("version count %d >= %d", result.VersionCount, thresh.MinVersionCount)
	}

	return false, fmt.Sprintf("downloads %d < %d and version count %d < %d",
		result.Downloads, thresh.MinDownloads, result.VersionCount, thresh.MinVersionCount)
}
