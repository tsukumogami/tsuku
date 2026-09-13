package project

import (
	"io/fs"
	"os"
	"path/filepath"
)

// rootUID is the one owner other than the invoking user that ownership alone
// accepts. Only an administrator can produce a root-owned file, so a
// root-owned config is one the machine's owner put there.
const rootUID uint32 = 0

// trustDecision is the outcome of judging one candidate config.
type trustDecision struct {
	reason string
	remedy string
}

// belowResolvedHome reports whether path sits strictly below a home directory
// that qualifies to switch the rule off.
//
// The early return this gates is the rule's off switch, so its preconditions
// carry as much weight as the clauses do. HOME is an environment variable, and
// anyone who can write a shell profile or a directory-local environment file
// can set it: HOME=/ must not disable the rule for the whole filesystem.
//
// A home qualifies only when it is set, resolves, is not the filesystem root,
// and is owned by the invoking user. Anything else and the clauses apply.
func belowResolvedHome(env DiscoveryEnv, path string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	resolved, err := env.EvalSymlinks(home)
	if err != nil {
		return false
	}
	resolved = filepath.Clean(resolved)
	if resolved == string(filepath.Separator) {
		return false
	}
	meta, err := env.Lstat(resolved)
	if err != nil || meta.UID != env.UID {
		return false
	}

	// Strictly below: a config at the home directory itself is not reached by
	// the walk anyway, since home is a ceiling.
	rel, err := filepath.Rel(resolved, filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && !hasParentPrefix(rel)
}

func hasParentPrefix(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)
}

// judge applies the three clauses to one candidate.
//
// All three must pass. They are not alternatives and none of them
// short-circuits the others: the ownership clause has an internal shortcut, and
// it is a shortcut past the ancestor chain alone.
//
// meta describes the object being judged; dir is the directory the clauses
// about surroundings apply to. For a symlinked config this runs twice, once at
// the link's own location and once at the target's, with checkMode false at the
// link -- see readThroughSymlink for why.
func judge(env DiscoveryEnv, meta FileMeta, dir string, checkMode bool) *trustDecision {
	if d := judgeOwnership(env, meta, dir); d != nil {
		return d
	}
	if d := judgeDirectory(env, dir); d != nil {
		return d
	}
	if checkMode {
		if d := judgeFileMode(meta); d != nil {
			return d
		}
	}
	return nil
}

// judgeOwnership accepts a config the invoking user or root owns, and accepts a
// third party's only when nobody else could have put it there.
//
// The chain is the whole rule. Two layouts decide it and they are
// ownership-identical: a checkout at /srv/app owned by another non-root user
// under a root-owned 0755 parent must be found, and a directory another user
// pre-created under a world-writable parent must be refused. What separates
// them sits above the config and is a permission bit -- only an administrator
// could have created /srv, while anyone could have created /tmp/build.
//
// Each writable ancestor must be owned by *the config's own owner*, not merely
// by somebody the rule otherwise trusts. /tmp is root-owned and world-writable,
// so accepting a root-owned writable ancestor accepts every config planted
// under it, which is the reported attack. No ancestor is exempted for carrying
// the sticky bit either, for the same reason: /tmp is sticky.
func judgeOwnership(env DiscoveryEnv, meta FileMeta, dir string) *trustDecision {
	if meta.UID == env.UID || meta.UID == rootUID {
		return nil
	}

	for _, ancestor := range env.Ancestors(dir) {
		am, err := env.Lstat(ancestor)
		if err != nil {
			// Cannot determine, so it fails -- the same treatment
			// configPermissionCondition gives an unreadable path.
			return &trustDecision{
				reason: "is owned by another user and a directory above it cannot be examined",
				remedy: "Take ownership of the file, or run as the user that owns it.",
			}
		}
		if am.Mode&0o022 == 0 {
			continue
		}
		if am.UID != meta.UID {
			return &trustDecision{
				reason: "is owned by another user, below a directory that anyone can write to",
				remedy: "Take ownership of the file, or move the checkout somewhere only its owner can write.",
			}
		}
	}
	return nil
}

// judgeDirectory refuses a config in a directory everyone can write to unless
// the sticky bit is set.
//
// What the sticky bit governs is unlinking, not creating, and that is the gap:
// without it an attacker can remove the user's config and put something else at
// the path, including a hard link to one of the user's own files, which every
// ownership check then accepts because the owner is genuinely the user. With it
// they cannot remove the user's file, so what is at the path is either the
// user's or carries the attacker's own ownership, which the first clause judges.
// The hard link an attacker can still create in a sticky directory is a
// residual, recorded in the design; this clause narrows the exposure rather
// than closing it.
//
// It reads the world bit only. A group-writable directory is the ordinary
// result of a umask of 002, and refusing those would refuse ordinary
// repositories on ordinary machines.
func judgeDirectory(env DiscoveryEnv, dir string) *trustDecision {
	meta, err := env.Lstat(dir)
	if err != nil {
		return &trustDecision{
			reason: "sits in a directory that cannot be examined",
			remedy: "Check the permissions on the directory holding it.",
		}
	}
	if meta.Mode&0o002 != 0 && meta.Mode&fs.ModeSticky == 0 {
		return &trustDecision{
			reason: "sits in a directory that anyone can write to and delete from",
			remedy: "Move the checkout somewhere only you can write, or set the sticky bit on the directory.",
		}
	}
	return nil
}

// judgeFileMode refuses a config anyone can rewrite in place.
//
// An in-place rewrite leaves no trace in any metadata an ownership check reads:
// the owner does not change, the inode does not change, only the bytes do.
//
// This is the clause that refuses a checkout on a Windows drive mounted without
// the metadata option, where the reported mode is translated from Windows
// permissions rather than stored. The remedy names the mount option as well as
// chmod, because chmod alone cannot fix that case.
func judgeFileMode(meta FileMeta) *trustDecision {
	if meta.Mode&0o002 == 0 {
		return nil
	}
	return &trustDecision{
		reason: "can be written by anyone",
		remedy: "Run chmod o-w on it. On a Windows drive mounted without metadata, " +
			"remount with the metadata option instead: chmod alone cannot change what that mount reports.",
	}
}
