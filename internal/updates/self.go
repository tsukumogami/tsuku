package updates

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/buildinfo"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/httputil"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/version"
)

const (
	// SelfToolName is the tool name used for tsuku self-update cache entries.
	SelfToolName = "tsuku"

	// SelfRepo is the GitHub repository for tsuku releases.
	SelfRepo = "tsukumogami/tsuku"

	// SelfUpdateLockFile is the advisory lock file for concurrent self-update dedup.
	SelfUpdateLockFile = ".self-update.lock"
)

// IsSelfUpdate returns true if the entry represents a tsuku self-update check.
func IsSelfUpdate(entry *UpdateCheckEntry) bool {
	return entry.Tool == SelfToolName
}

// IsDevBuild returns true if the version string indicates a non-release build.
// This includes "dev", "unknown", "dev-<hash>" from local builds, and Go
// pseudo-versions (e.g., "v0.7.1-0.20260401194153-abc123"). Pre-release
// tags like "v1.0.0-rc.1" are NOT considered dev builds.
func IsDevBuild(ver string) bool {
	if ver == "dev" || ver == "unknown" || strings.HasPrefix(ver, "dev-") {
		return true
	}
	// Go pseudo-versions: v0.0.0-YYYYMMDDHHMMSS-hash or v0.7.1-0.YYYYMMDDHHMMSS-hash
	// The distinguishing pattern is "-0." or "-" followed by 14 digits (timestamp).
	stripped := strings.TrimPrefix(ver, "v")
	if idx := strings.Index(stripped, "-"); idx >= 0 {
		suffix := stripped[idx+1:]
		// Pseudo-version: starts with "0." (base version prefix) or 14+ digit timestamp
		if strings.HasPrefix(suffix, "0.") {
			return true
		}
		// Also catch direct timestamp format (v0.0.0-20260401...)
		if len(suffix) >= 14 && isAllDigits(suffix[:14]) {
			return true
		}
	}
	return false
}

// isAllDigits returns true if every byte in s is an ASCII digit.
func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// CheckAndApplySelf checks for a newer tsuku release and applies it if auto-apply
// is enabled. Dev builds are skipped. The check result is always cached regardless
// of whether an update is applied.
func CheckAndApplySelf(ctx context.Context, cfg *config.Config, userCfg *userconfig.Config, cacheDir string, resolver *version.Resolver) error {
	if !userCfg.UpdatesSelfUpdate() {
		return nil
	}

	currentVersion := buildinfo.Version()
	if IsDevBuild(currentVersion) {
		return nil
	}

	provider := version.NewGitHubProvider(resolver, SelfRepo)

	latest, err := provider.ResolveLatest(ctx)
	if err != nil {
		return fmt.Errorf("resolve latest tsuku version: %w", err)
	}

	// Normalize both versions by stripping "v" prefix
	normalizedCurrent := strings.TrimPrefix(currentVersion, "v")
	normalizedLatest := strings.TrimPrefix(latest.Version, "v")

	// Write cache entry regardless of comparison outcome
	now := time.Now()
	entry := &UpdateCheckEntry{
		Tool:          SelfToolName,
		ActiveVersion: normalizedCurrent,
		LatestOverall: normalizedLatest,
		Source:        provider.SourceDescription(),
		CheckedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
	_ = WriteEntry(cacheDir, entry) // best-effort, matching tool check pattern

	// If current is equal to or newer than latest, nothing to do
	cmp := CompareSemver(normalizedCurrent, normalizedLatest)
	if cmp >= 0 {
		return nil
	}

	// Newer version available -- apply it.
	// The UpdatesSelfUpdate() check at function entry is the sole gate; this path
	// is independent of UpdatesAutoApplyEnabled() so that users can disable tool
	// auto-apply while keeping tsuku self-updates enabled.

	// Acquire non-blocking lock to prevent concurrent self-updates
	lockPath := filepath.Join(cacheDir, SelfUpdateLockFile)
	lock := install.NewFileLock(lockPath)
	acquired, err := lock.TryLockExclusive()
	if err != nil {
		return fmt.Errorf("try self-update lock: %w", err)
	}
	if !acquired {
		return nil // Another update in progress
	}
	defer func() { _ = lock.Unlock() }()

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("eval symlinks for executable: %w", err)
	}

	assetName := fmt.Sprintf("tsuku-%s-%s", runtime.GOOS, runtime.GOARCH)
	noticesDir := notices.NoticesDir(cfg.HomeDir)

	if applyErr := ApplySelfUpdate(ctx, exePath, latest.Tag, assetName); applyErr != nil {
		log.Default().Debug("self-update apply failed", "error", applyErr)
		return nil // Best-effort, don't propagate
	}

	// Write success notice
	notice := &notices.Notice{
		Tool:             SelfToolName,
		AttemptedVersion: normalizedLatest,
		Error:            "",
		Timestamp:        time.Now(),
		Shown:            false,
	}
	_ = notices.WriteNotice(noticesDir, notice)

	return nil
}

// ApplySelfUpdate downloads, verifies, and replaces the running tsuku binary.
// It uses an atomic rename with backup for crash safety.
func ApplySelfUpdate(ctx context.Context, exePath, tag, assetName string) error {
	checksumsURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/checksums.txt", SelfRepo, tag)
	binaryURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", SelfRepo, tag, assetName)

	client := httputil.NewSecureClient(httputil.ClientOptions{})

	// Download checksums.txt
	checksumsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return fmt.Errorf("create checksums request: %w", err)
	}
	checksumsResp, err := client.Do(checksumsReq)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	defer checksumsResp.Body.Close()

	if checksumsResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download checksums: HTTP %d", checksumsResp.StatusCode)
	}

	checksumsData, err := io.ReadAll(io.LimitReader(checksumsResp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	expectedHash, err := parseChecksumForAsset(checksumsData, assetName)
	if err != nil {
		return fmt.Errorf("parse checksum for %s: %w", assetName, err)
	}

	// Download binary to temp file in same directory as target
	binaryReq, err := http.NewRequestWithContext(ctx, http.MethodGet, binaryURL, nil)
	if err != nil {
		return fmt.Errorf("create binary request: %w", err)
	}
	binaryResp, err := client.Do(binaryReq)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer binaryResp.Body.Close()

	if binaryResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download binary: HTTP %d", binaryResp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), ".tsuku-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Compute SHA256 while writing (cap at 200MB to bound resource usage)
	const maxBinarySize = 200 << 20
	hasher := sha256.New()
	if _, err := io.Copy(tmpFile, io.TeeReader(io.LimitReader(binaryResp.Body, maxBinarySize), hasher)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write binary: %w", err)
	}
	tmpFile.Close()

	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if computedHash != expectedHash {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, computedHash)
	}

	// Preserve permissions from current binary
	info, err := os.Stat(exePath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("stat current binary: %w", err)
	}
	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp binary: %w", err)
	}

	// Atomic replace with backup
	_ = os.Remove(exePath + ".old") // Remove stale backup, ignore error
	if err := os.Rename(exePath, exePath+".old"); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Restore from backup
		_ = os.Rename(exePath+".old", exePath)
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}

// parseChecksumForAsset extracts the SHA256 hash for a given asset from
// a checksums.txt file. Each line has the format "{hash}  {filename}".
func parseChecksumForAsset(data []byte, assetName string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Format: "{hash}  {filename}" (two spaces between hash and filename)
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		if parts[1] == assetName {
			hash := strings.ToLower(parts[0])
			if len(hash) != 64 || !isHexString(hash) {
				return "", fmt.Errorf("invalid hash format for %s: %q", assetName, parts[0])
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("no checksum found for asset %s", assetName)
}

// isHexString returns true if s contains only lowercase hex characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// CompareSemver compares two clean semver strings (without "v" prefix).
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Only handles numeric segments (e.g., "1.2.3"). Non-numeric segments are
// treated as 0. Pre-release suffixes are not parsed -- callers should filter
// pre-release versions with IsDevBuild before comparing.
func CompareSemver(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			aNum, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bNum, _ = strconv.Atoi(bParts[i])
		}
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
	}
	return 0
}
