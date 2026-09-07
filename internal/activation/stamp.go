package activation

import (
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

// NoStateToken is the stamp recorded when installation state cannot be stat-ed
// for any reason -- it does not exist, the directory is unreadable, anything.
//
// It is deliberately a fixed non-empty literal. The short-circuit tests
// `stamp != ""`, so an empty token would mean the early exit never fires on a
// machine with no state.json at all -- a fresh install, or TSUKU_HOME pointed
// somewhere new -- and every prompt would re-parse and re-resolve forever,
// silently, on exactly the machines with nothing installed.
//
// It carries no error text and no path, because it is emitted into the
// environment and a path would put $TSUKU_HOME into an exported value.
const NoStateToken = "no-state"

// StateStamp returns a token summarizing installation state's on-disk identity,
// from a single os.Stat.
//
// It is mtime and size together, and size is load-bearing rather than
// belt-and-braces. Linux caches a file's timestamp per timer tick instead of
// reading the clock per write: measured on ordinary ext4, two back-to-back
// writes carry the identical nanosecond mtime 185 times out of 200. So an
// mtime-only stamp does not change when an install lands a tick after a
// prompt -- which is the exact case this exists to catch, and it is the common
// case rather than a coarse-filesystem curiosity. Size closes it free, since
// installing a version always adds a VersionState and so changes the length.
//
// Not the inode -- tsuku builds for Windows, which has no portable equivalent.
//
// Not a content hash: hashing is the read the stamp exists to avoid. os.Stat on
// a real state file costs microseconds against a prompt budget in milliseconds.
// The path comes from install.StatePath rather than being joined here. A local
// copy would keep working until installation state moved, and would then stat a
// file that does not exist -- which this function reports as NoStateToken, a
// value that never changes, so every prompt would short-circuit and no
// remediation would ever take effect. Silent, and the exact failure the stamp
// exists to prevent.
func StateStamp(cfg *config.Config) string {
	info, err := os.Stat(install.StatePath(cfg))
	if err != nil {
		return NoStateToken
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
}
