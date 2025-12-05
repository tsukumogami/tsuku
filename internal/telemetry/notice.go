package telemetry

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

const (
	// NoticeMarkerFile is the filename used to track if the notice has been shown.
	NoticeMarkerFile = "telemetry_notice_shown"

	// NoticeText is the message displayed to users on first run.
	NoticeText = `tsuku collects anonymous usage statistics to improve the tool.
No personal information is collected. See: https://tsuku.dev/telemetry

To opt out: tsuku config set telemetry false
         or export TSUKU_NO_TELEMETRY=1
`
)

// ShowNoticeIfNeeded displays the telemetry notice on first run.
// It writes to stderr and creates a marker file to prevent future displays.
// Returns silently on any error (file permissions, etc.).
func ShowNoticeIfNeeded() {
	// Don't show notice if telemetry is disabled via env var
	if os.Getenv(EnvNoTelemetry) != "" {
		return
	}

	// Don't show notice if telemetry is disabled via config
	userCfg, err := userconfig.Load()
	if err == nil && !userCfg.Telemetry {
		return
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		return // Silent failure
	}

	showNoticeIfNeeded(cfg.HomeDir, os.Stderr)
}

// showNoticeIfNeeded is the internal implementation that accepts a home directory
// and output writer for testability. It displays the notice and creates a marker file.
func showNoticeIfNeeded(homeDir string, output io.Writer) {
	markerPath := filepath.Join(homeDir, NoticeMarkerFile)

	// Check if marker file exists
	if _, err := os.Stat(markerPath); err == nil {
		return // Already shown
	}

	// Show notice to output
	_, _ = fmt.Fprint(output, NoticeText)

	// Create marker file (ensure directory exists)
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return // Silent failure
	}

	// Create empty marker file
	f, err := os.Create(markerPath)
	if err != nil {
		return // Silent failure
	}
	f.Close()
}
