package actions

import (
	"bufio"
	"bytes"
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/sonameindex"
)

// scanResult captures the outcome of a SONAME completeness scan over a
// bottle's binaries. Auto-included entries are passed by Execute to the
// chain walks (fixDylibRpathChain / fixElfRpathChain) along with the
// recipe's declared RuntimeDependencies.
type scanResult struct {
	// AutoInclude is the de-duplicated list of chainEntry instances the
	// scanner produced for under-declared deps (i.e. NEEDED SONAMES that
	// map to a tsuku library recipe but are not in
	// ctx.Dependencies.RuntimeDependencies). Entry order is deterministic
	// (sorted by recipe name).
	AutoInclude []chainEntry
}

// runSonameCompletenessScan walks every binary under ctx.WorkDir/{bin,lib},
// extracts its NEEDED SONAMES, and classifies each one against the SONAME
// index. The classification produces, in priority order:
//
//  1. System library (resolves via ldconfig on Linux, /usr/lib or
//     /System/Library on macOS) — no action.
//  2. Maps to a tsuku library that is already in
//     ctx.Dependencies.RuntimeDependencies — no action; the chain walk
//     handles it.
//  3. Maps to a tsuku library that is NOT in RuntimeDependencies — log a
//     warning and append the dep to scanResult.AutoInclude so the chain
//     walk picks it up.
//  4. No tsuku recipe ships this SONAME — log a coverage gap. SONAMES on
//     the static known-gap allowlist (see sonameindex.IsKnownGap) are
//     downgraded to debug-level logging so install output stays clean
//     until the corresponding library recipes land.
//
// The scanner does NOT mutate ctx.Dependencies. The auto-included slice
// lives entirely in Execute scope. Callers MUST union RuntimeDependencies
// with scanResult.AutoInclude when emitting RPATH entries.
//
// If ctx.SonameIndex is nil the scanner is a no-op — classification cannot
// proceed without an index, and the design forbids guessing. Production
// call sites populate the index at plan-generation time.
//
// Parse failures on individual binaries (corrupted bottles, unknown formats)
// are logged at warn-level and the binary is skipped; the scan continues
// over the rest of the tree rather than treating the parse failure as
// "no NEEDED entries" (which would silently swallow real coverage gaps).
func runSonameCompletenessScan(ctx *ExecutionContext, reporter progress.Reporter) (*scanResult, error) {
	result := &scanResult{}
	if ctx == nil {
		return result, nil
	}
	if ctx.SonameIndex == nil {
		// No index → no classification possible. Scanner exits cleanly so
		// the rest of the relocate phase still runs. Non-fatal: the chain
		// walk degrades to RuntimeDependencies-only.
		return result, nil
	}

	binaries := collectScanCandidates(ctx.WorkDir)
	if len(binaries) == 0 {
		return result, nil
	}

	platform := sonameindex.PlatformLinux
	if runtime.GOOS == "darwin" {
		platform = sonameindex.PlatformDarwin
	}

	declared := make(map[string]bool, len(ctx.Dependencies.RuntimeDependencies))
	for _, name := range ctx.Dependencies.RuntimeDependencies {
		declared[name] = true
	}

	systemSet := loadSystemSonameSet()

	// Track auto-includes by recipe name to dedupe across binaries.
	autoNames := make(map[string]bool)
	// Track coverage gaps to dedupe log lines (one warning per missing
	// SONAME, not per binary).
	loggedGaps := make(map[string]bool)
	loggedUnderDeclared := make(map[string]bool)

	for _, bin := range binaries {
		needed, err := readNeededSonames(bin)
		if err != nil {
			reporter.Warn("   SONAME scan: skipping %s: %v", filepath.Base(bin), err)
			continue
		}
		for _, soname := range needed {
			if isSystemSoname(platform, soname, systemSet) {
				continue
			}
			provider, ok := ctx.SonameIndex.Lookup(platform, soname)
			if !ok {
				if loggedGaps[soname] {
					continue
				}
				loggedGaps[soname] = true
				if sonameindex.IsKnownGap(soname) {
					reporter.Log("   SONAME coverage gap (known): %s has no tsuku library provider", soname)
				} else {
					reporter.Warn("   SONAME coverage gap: %s (referenced by %s) has no tsuku library provider; install may fail on minimal containers",
						soname, filepath.Base(bin))
				}
				continue
			}
			if declared[provider.Recipe] {
				// Author already declared this dep; chain walk handles it.
				continue
			}
			if autoNames[provider.Recipe] {
				// Already queued for auto-include from an earlier binary.
				continue
			}
			autoNames[provider.Recipe] = true
			if !loggedUnderDeclared[provider.Recipe] {
				reporter.Warn("   recipe under-declared: binary needs %s from %s; not in runtime_dependencies (auto-included)",
					soname, provider.Recipe)
				loggedUnderDeclared[provider.Recipe] = true
			}
		}
	}

	if len(autoNames) == 0 {
		return result, nil
	}

	// Resolve each auto-included recipe to (name, version) for the chain
	// walk. Version comes from ctx.Dependencies.Runtime if available
	// (the dep is already resolved as a transitive runtime dep of the
	// declared graph), otherwise we glob $LibsDir/<name>-* for the highest
	// installed version. An auto-included entry that resolves to neither
	// is skipped with a warning rather than emitted as a known-bad
	// "-latest" path that does not exist on disk.
	names := make([]string, 0, len(autoNames))
	for name := range autoNames {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		v, ok := resolveAutoIncludeVersion(ctx, name)
		if !ok {
			reporter.Warn("   auto-include skipped: %s has no resolved version and no installed sibling under %s",
				name, ctx.LibsDir)
			continue
		}
		result.AutoInclude = append(result.AutoInclude, chainEntry{name: name, version: v})
	}

	return result, nil
}

// collectScanCandidates walks workDir/{bin,lib} for regular files (skipping
// symlinks and directories). The scanner inspects every candidate and
// silently skips ones whose magic bytes don't match a recognized binary
// format — readNeededSonames handles that filtering rather than the walk.
func collectScanCandidates(workDir string) []string {
	if workDir == "" {
		return nil
	}
	var binaries []string
	for _, sub := range []string{"bin", "lib"} {
		root := filepath.Join(workDir, sub)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			binaries = append(binaries, p)
			return nil
		})
	}
	return binaries
}

// readNeededSonames returns the NEEDED SONAMES for the binary at path.
// Linux uses debug/elf (parses DT_NEEDED entries directly), macOS shells
// out to otool -L (and parses install-name lines, which are the practical
// equivalent of NEEDED entries for the purposes of the scan).
//
// Files that aren't recognized binaries return (nil, nil) — the scanner
// loop treats that as a clean no-op for that file. Files that look like
// the expected format but fail to parse return a non-nil error so the
// caller can log a warn-level skip line.
func readNeededSonames(path string) ([]string, error) {
	magic, err := readMagic(path)
	if err != nil {
		return nil, nil // unreadable file — let the caller skip it silently
	}
	if isELFMagic(magic) {
		return readELFNeeded(path)
	}
	if isMachOMagic(magic) {
		return readMachONeeded(path)
	}
	// Not a recognized binary format — text file, archive, etc.
	return nil, nil
}

// readMagic returns the first four bytes of the file at path, or an error
// if the file is shorter than four bytes or unreadable.
func readMagic(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return nil, err
	}
	return magic, nil
}

// isELFMagic reports whether the four-byte magic header identifies an ELF
// binary.
func isELFMagic(magic []byte) bool {
	return len(magic) >= 4 && magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F'
}

// isMachOMagic reports whether the four-byte magic header identifies any
// of the Mach-O variants (32/64-bit, big/little-endian, fat).
func isMachOMagic(magic []byte) bool {
	if len(magic) < 4 {
		return false
	}
	return bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) ||
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) ||
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) ||
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) ||
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) ||
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca})
}

// readELFNeeded returns the DT_NEEDED entries from the dynamic section of
// the ELF binary at path. If the file is not a parseable ELF binary or
// has no dynamic section, returns a non-nil error so the caller can log
// a warning rather than misclassifying it as "no NEEDED entries".
func readELFNeeded(path string) ([]string, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("elf.Open: %w", err)
	}
	defer func() { _ = f.Close() }()
	needed, err := f.DynString(elf.DT_NEEDED)
	if err != nil {
		return nil, fmt.Errorf("DynString(DT_NEEDED): %w", err)
	}
	return needed, nil
}

// readMachONeeded shells out to otool -L to enumerate the install-name
// references for a Mach-O binary. otool's -L output is one install-name
// per line (after a header line that names the binary). The returned
// SONAMES are the basenames of those install-names, matching the form the
// SONAME index uses.
//
// otool ships with macOS Xcode tools and is the standard install-name
// inspection tool there. If otool isn't found, the scanner returns a
// best-effort error so the caller can skip the binary with a warning.
func readMachONeeded(path string) ([]string, error) {
	otool, err := exec.LookPath("otool")
	if err != nil {
		return nil, fmt.Errorf("otool not found: %w", err)
	}
	cmd := exec.Command(otool, "-L", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("otool -L: %w", err)
	}
	var sonames []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			// otool prints the binary path on the first line. Skip it.
			first = false
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "/path/to/lib.dylib (compatibility version ...)"
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		ref := fields[0]
		if ref == "" {
			continue
		}
		sonames = append(sonames, filepath.Base(ref))
	}
	return sonames, nil
}

// systemSonames is a per-process cache of the system runtime linker's
// known SONAMES. On Linux it's populated from `ldconfig -p`; on macOS it's
// left empty because the macOS dynamic linker resolves /usr/lib/* and
// /System/Library/* via path patterns rather than a cache, and that
// pattern matching is handled by isSystemSoname directly.
var (
	systemSonamesOnce sync.Once
	systemSonames     map[string]bool
)

// loadSystemSonameSet returns a snapshot of the SONAMES the system
// runtime linker knows how to resolve. The set is computed once per
// process; subsequent calls return the cached set. On Linux this shells
// out to ldconfig -p. On macOS the set is empty (path-pattern checks in
// isSystemSoname carry the load there).
func loadSystemSonameSet() map[string]bool {
	systemSonamesOnce.Do(func() {
		if runtime.GOOS != "linux" {
			systemSonames = map[string]bool{}
			return
		}
		systemSonames = parseLdconfigOutput(runLdconfig())
	})
	return systemSonames
}

// runLdconfig invokes `ldconfig -p` and returns the raw output. Errors
// (ldconfig missing, invocation failure) yield an empty result so callers
// degrade to "nothing classifies as a system library", which is the safe
// direction (auto-include extras rather than skip needed deps).
func runLdconfig() []byte {
	ldconfig, err := exec.LookPath("ldconfig")
	if err != nil {
		// /sbin/ldconfig is on most distros even when not on PATH.
		ldconfig = "/sbin/ldconfig"
		if _, err := os.Stat(ldconfig); err != nil {
			return nil
		}
	}
	cmd := exec.Command(ldconfig, "-p")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return out
}

// parseLdconfigOutput returns the SONAMES listed in ldconfig -p output.
// ldconfig output looks like:
//
//	1234 libs found in cache `/etc/ld.so.cache'
//		libfoo.so.1 (libc6,x86-64) => /lib/x86_64-linux-gnu/libfoo.so.1
//
// We extract the leading SONAME (the token before " ("). The first line
// is the count header, which contains no parentheses and is naturally
// filtered out by the parser.
func parseLdconfigOutput(out []byte) map[string]bool {
	set := map[string]bool{}
	if len(out) == 0 {
		return set
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		paren := strings.Index(line, " (")
		if paren <= 0 {
			continue
		}
		soname := strings.TrimSpace(line[:paren])
		if soname == "" {
			continue
		}
		set[soname] = true
	}
	return set
}

// isSystemSoname reports whether soname resolves via the system runtime
// linker on the given platform. On Linux this checks the ldconfig -p set.
// On macOS it pattern-matches the well-known system-library locations
// (the macOS dynamic linker resolves /usr/lib/* and /System/Library/*
// without consulting any cache).
func isSystemSoname(platform sonameindex.Platform, soname string, systemSet map[string]bool) bool {
	if platform == sonameindex.PlatformDarwin {
		// macOS system libraries: install-names usually live under /usr/lib
		// or /System/Library. The scanner records install-name basenames,
		// so we have to recognize the system shapes by name pattern: the
		// libSystem family (libSystem.B.dylib, libSystem.dylib) and the
		// "/usr/lib/libfoo.dylib" install names whose basename is shipped
		// only by the OS. Without an authoritative list, the conservative
		// move is to treat any libSystem* name as system and let the index
		// classify everything else. False negatives here just mean an
		// extra index lookup, never a misclassification.
		if strings.HasPrefix(soname, "libSystem") {
			return true
		}
		return false
	}
	return systemSet[soname]
}

// resolveAutoIncludeVersion returns a version string and ok flag for an
// auto-included recipe. Resolution delegates to resolveRuntimeDepVersion
// so the chain walk and the SONAME scan agree on which version of a dep
// to pin into the emitted RPATH.
//
// Returns ("", false) when neither the executor's Runtime map nor a glob
// over ctx.LibsDir/<name>-* yields a real version. Callers must NOT emit
// an RPATH for an unresolved entry — the historical "-latest" fallback
// produced paths that did not exist on disk and broke binaries at
// runtime ("cannot open shared object file").
func resolveAutoIncludeVersion(ctx *ExecutionContext, name string) (string, bool) {
	return resolveRuntimeDepVersion(ctx, name)
}
