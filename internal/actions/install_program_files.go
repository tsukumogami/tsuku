package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// InstallProgramFilesAction copies files from a tool's install directory into that
// tool's stable data directory ($TSUKU_HOME/data/<tool>).
//
// It exists for tools that keep program and data in one root. nvm is the case in hand:
// `nvm exec` runs "${NVM_DIR}/nvm-exec", and nvm-exec re-sources nvm.sh from its own
// directory, so both files have to sit beside the user's Node versions. Point NVM_DIR at
// a stable root without them and `nvm exec` and `nvm run` fail with 127 while install,
// ls, use, which and alias all keep working — a partial break quiet enough to ship
// unnoticed.
//
// Copies rather than symlinks, deliberately. Activate, rollback and removal-promotion
// never re-run the post-install phase, so a link placed by a recipe tracks the
// last-installed version rather than the active one and dangles once that version is
// reaped. A copy cannot dangle, and copying on every install makes placement and
// repointing the same idempotent operation, so there is no separate refresh step a
// future change can forget.
//
// There is deliberately no destination parameter. See Execute.
type InstallProgramFilesAction struct{ BaseAction }

// Name returns the action name.
func (a *InstallProgramFilesAction) Name() string { return "install_program_files" }

// IsDeterministic returns true: the action reads the tool's own install directory and
// nothing else.
func (InstallProgramFilesAction) IsDeterministic() bool { return true }

// DefaultPhase runs this action after the atomic rename.
//
// The source is ctx.ToolInstallDir, which is only populated once the tool's permanent
// directory exists. Copying out of the staging directory during the install phase would
// produce identical bytes but would write into a stable path on behalf of an install
// that can still fail.
func (InstallProgramFilesAction) DefaultPhase() string { return "post-install" }

// Preflight validates parameters without side effects.
func (a *InstallProgramFilesAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	files, ok := GetStringSlice(params, "files")
	if !ok || len(files) == 0 {
		result.AddError("install_program_files requires a non-empty 'files' parameter")
		return result
	}

	for _, f := range files {
		if f == "" {
			result.AddError("install_program_files: 'files' entry may not be empty")
			continue
		}
		if filepath.IsAbs(f) {
			result.AddErrorf("install_program_files: %q must be relative to the tool install directory", f)
		}
		if hasParentTraversal(f) {
			result.AddErrorf("install_program_files: %q may not contain %q", f, "..")
		}
	}

	return result
}

// hasParentTraversal reports a ".." component in a slash- or backslash-separated path.
func hasParentTraversal(p string) bool {
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

// Execute copies each named file into $TSUKU_HOME/data/<tool>.
//
// Parameters:
//   - files (required): paths relative to the tool install directory
//
// The destination is computed here, not named by the recipe. That is a security
// property rather than a convenience: a recipe-supplied directory anywhere under
// $TSUKU_HOME would reach share/shell.d, which RebuildShellCache concatenates into the
// script the user's shell sources — and the selection machinery includes a fragment it
// has no record of rather than excluding it. Since the bytes being copied come out of a
// downloaded archive, that would let a recipe put arbitrary code into a login shell, and
// because this action ignores --no-shell-init it would bypass the one flag a user has
// for opting out. With the destination fixed there is no recipe-controlled path left to
// validate.
func (a *InstallProgramFilesAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	files, ok := GetStringSlice(params, "files")
	if !ok || len(files) == 0 {
		return fmt.Errorf("install_program_files requires a non-empty 'files' parameter")
	}

	if ctx.ToolInstallDir == "" {
		return fmt.Errorf("install_program_files: tool install directory is not available (this action must run in the post-install phase)")
	}

	destDir, err := dataDir(ctx)
	if err != nil {
		return fmt.Errorf("install_program_files: %w", err)
	}

	// nvm creates its own data tree, so this only has to exist for our own writes.
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return fmt.Errorf("install_program_files: failed to create data directory: %w", err)
	}

	// The archive extractor's containment checks are lexical, so a symlink in a release
	// tarball can point outside the tool directory. Resolve before trusting.
	resolvedRoot, err := filepath.EvalSymlinks(ctx.ToolInstallDir)
	if err != nil {
		return fmt.Errorf("install_program_files: failed to resolve tool install directory: %w", err)
	}

	for _, rel := range files {
		if err := copyProgramFile(ctx, resolvedRoot, destDir, rel); err != nil {
			return fmt.Errorf("install_program_files: %w", err)
		}
	}

	return nil
}

// copyProgramFile copies one file from the resolved tool directory into destDir.
func copyProgramFile(ctx *ExecutionContext, resolvedRoot, destDir, rel string) error {
	src := filepath.Join(resolvedRoot, rel)

	resolvedSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("%q: %w", rel, err)
	}
	if !isWithin(resolvedSrc, resolvedRoot) {
		return fmt.Errorf("%q resolves to %q, outside the tool install directory", rel, resolvedSrc)
	}

	// O_NOFOLLOW plus fstat on the handle: the type check and the read then see the same
	// object, which a stat-then-open pair cannot guarantee.
	in, err := os.OpenFile(resolvedSrc, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("%q: %w", rel, err)
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("%q: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", rel)
	}

	// Derived from the execute bit rather than carried over from the tar header, so a
	// crafted archive cannot choose the mode of a file tsuku writes.
	mode := os.FileMode(0644)
	if info.Mode().Perm()&0111 != 0 {
		mode = 0755
	}

	base := filepath.Base(rel)
	dest := filepath.Join(destDir, base)
	tmp := dest + ".tsuku-tmp"

	// O_EXCL|O_NOFOLLOW so a pre-planted symlink at the temp path cannot redirect the
	// write. Removing first keeps a crashed previous run from wedging this one.
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%q: %w", rel, err)
	}
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return fmt.Errorf("%q: %w", rel, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("%q: %w", rel, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%q: %w", rel, err)
	}
	// O_CREATE honors the umask, so set the mode explicitly.
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%q: %w", rel, err)
	}
	// Rename last: a concurrent `nvm exec` never observes a half-written file.
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%q: %w", rel, err)
	}

	recordProgramFileCleanup(ctx, dest)
	return nil
}

// isWithin reports whether path is root or lies beneath it.
func isWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// recordProgramFileCleanup records a delete_file cleanup for a placed program file.
//
// The file itself is tsuku's, so removing the tool takes it. The user's data alongside it
// is never recorded and so is never reachable from a cleanup action — which matters
// because garbage collection executes cleanup actions from a background process.
func recordProgramFileCleanup(ctx *ExecutionContext, dest string) {
	tsukuHome := filepath.Dir(ctx.ToolsDir)
	rel, err := filepath.Rel(tsukuHome, dest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	for i := range ctx.CleanupActions {
		if ctx.CleanupActions[i].Path == rel {
			return
		}
	}
	ctx.CleanupActions = append(ctx.CleanupActions, CleanupAction{
		Action: "delete_file",
		Path:   rel,
	})
}
