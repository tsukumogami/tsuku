package updates

import (
	"os"
	"time"
)

// OOCThrottleDuration is the minimum time between out-of-channel
// notifications for the same tool.
const OOCThrottleDuration = 7 * 24 * time.Hour

// OOCFilePrefix is the dotfile prefix for per-tool throttle files.
const OOCFilePrefix = ".ooc-"

// IsOOCThrottled returns true if the tool's out-of-channel notification
// was shown within the last 7 days. The now parameter enables clock
// injection for testing.
func IsOOCThrottled(cacheDir, toolName string, now time.Time) bool {
	path := cacheDir + "/" + OOCFilePrefix + toolName
	info, err := os.Stat(path)
	if err != nil {
		return false // file doesn't exist, not throttled
	}
	return now.Sub(info.ModTime()) < OOCThrottleDuration
}

// TouchOOCThrottle creates or updates the throttle file for the tool.
func TouchOOCThrottle(cacheDir, toolName string) error {
	path := cacheDir + "/" + OOCFilePrefix + toolName
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
