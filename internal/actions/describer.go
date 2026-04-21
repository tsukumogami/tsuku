package actions

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/progress"
)

// ActionDescriber is an optional interface that actions implement to provide
// a human-readable status message for use in progress displays.
//
// StatusMessage is called with the action's params map before the action
// executes. It returns a short phrase suitable for a spinner or progress line
// (e.g., "Downloading gh_2.66.1_linux_amd64.tar.gz"). Implementations must
// sanitize any recipe-sourced string with progress.SanitizeDisplayString to
// prevent ANSI injection.
//
// Returning "" signals that no message is available; the executor falls back
// to the action name in that case.
type ActionDescriber interface {
	StatusMessage(params map[string]interface{}) string
}

// StatusMessage implements ActionDescriber for DownloadFileAction.
// Returns "Downloading <basename(url)>" with file size appended when available.
func (a *DownloadFileAction) StatusMessage(params map[string]interface{}) string {
	url, ok := GetString(params, "url")
	if !ok || url == "" {
		return ""
	}
	base := filepath.Base(url)
	// Strip query parameters from basename.
	if idx := strings.Index(base, "?"); idx != -1 {
		base = base[:idx]
	}
	base = progress.SanitizeDisplayString(base)
	if base == "" {
		return ""
	}

	msg := "Downloading " + base

	// Append human-readable size when provided.
	if size, ok := params["size"]; ok {
		switch v := size.(type) {
		case int:
			if v > 0 {
				msg += fmt.Sprintf(" (%s)", formatBytes(int64(v)))
			}
		case int64:
			if v > 0 {
				msg += fmt.Sprintf(" (%s)", formatBytes(v))
			}
		case float64:
			if v > 0 {
				msg += fmt.Sprintf(" (%s)", formatBytes(int64(v)))
			}
		}
	}

	return msg
}

// formatBytes returns a human-readable representation of a byte count.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n := n / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// StatusMessage implements ActionDescriber for ExtractAction.
// Returns "Extracting <archive>".
func (a *ExtractAction) StatusMessage(params map[string]interface{}) string {
	archive, ok := GetString(params, "archive")
	if !ok || archive == "" {
		return ""
	}
	archive = progress.SanitizeDisplayString(archive)
	if archive == "" {
		return ""
	}
	return "Extracting " + archive
}

// StatusMessage implements ActionDescriber for InstallBinariesAction.
// Returns "Installing <binary names>".
func (a *InstallBinariesAction) StatusMessage(params map[string]interface{}) string {
	outputs := a.getOutputsParam(params)
	if len(outputs) == 0 {
		return ""
	}

	var names []string
	for _, item := range outputs {
		switch v := item.(type) {
		case string:
			names = append(names, filepath.Base(v))
		case map[string]interface{}:
			if dest, ok := v["dest"].(string); ok && dest != "" {
				names = append(names, filepath.Base(dest))
			} else if src, ok := v["src"].(string); ok && src != "" {
				names = append(names, filepath.Base(src))
			}
		}
	}

	if len(names) == 0 {
		return ""
	}

	joined := progress.SanitizeDisplayString(strings.Join(names, ", "))
	if joined == "" {
		return ""
	}
	return "Installing " + joined
}

// StatusMessage implements ActionDescriber for ConfigureMakeAction.
// Returns "Building <source_dir>".
func (a *ConfigureMakeAction) StatusMessage(params map[string]interface{}) string {
	sourceDir, ok := GetString(params, "source_dir")
	if !ok || sourceDir == "" {
		return ""
	}
	name := progress.SanitizeDisplayString(filepath.Base(sourceDir))
	if name == "" {
		return ""
	}
	return "Building " + name
}

// StatusMessage implements ActionDescriber for CargoBuildAction.
// Returns "Building <crate>@<version>" when crate is available, or a generic message.
func (a *CargoBuildAction) StatusMessage(params map[string]interface{}) string {
	crate, hasCrate := GetString(params, "crate")
	ver, hasVer := GetString(params, "version")

	if hasCrate && crate != "" {
		crate = progress.SanitizeDisplayString(crate)
		if crate == "" {
			return ""
		}
		if hasVer && ver != "" {
			ver = progress.SanitizeDisplayString(ver)
			return fmt.Sprintf("Building %s@%s", crate, ver)
		}
		return "Building " + crate
	}

	// source_dir mode
	sourceDir, ok := GetString(params, "source_dir")
	if !ok || sourceDir == "" {
		return ""
	}
	name := progress.SanitizeDisplayString(filepath.Base(sourceDir))
	if name == "" {
		return ""
	}
	return "Building " + name
}

// StatusMessage implements ActionDescriber for GoBuildAction.
// Returns "Building <module>@<version>".
func (a *GoBuildAction) StatusMessage(params map[string]interface{}) string {
	module, ok := GetString(params, "module")
	if !ok || module == "" {
		return ""
	}
	// Use only the last path segment for brevity.
	module = filepath.Base(module)
	module = progress.SanitizeDisplayString(module)
	if module == "" {
		return ""
	}

	ver, hasVer := GetString(params, "version")
	if hasVer && ver != "" {
		ver = progress.SanitizeDisplayString(ver)
		return fmt.Sprintf("Building %s@%s", module, ver)
	}
	return "Building " + module
}

// StatusMessage implements ActionDescriber for CargoInstallAction.
// Returns "cargo install <crate>@<version>".
func (a *CargoInstallAction) StatusMessage(params map[string]interface{}) string {
	crate, ok := GetString(params, "crate")
	if !ok || crate == "" {
		return ""
	}
	crate = progress.SanitizeDisplayString(crate)
	if crate == "" {
		return ""
	}
	return "cargo install " + crate
}

// StatusMessage implements ActionDescriber for NpmInstallAction.
// Returns "npm install <package>".
func (a *NpmInstallAction) StatusMessage(params map[string]interface{}) string {
	pkg, ok := GetString(params, "package")
	if !ok || pkg == "" {
		return ""
	}
	pkg = progress.SanitizeDisplayString(pkg)
	if pkg == "" {
		return ""
	}
	return "npm install " + pkg
}

// StatusMessage implements ActionDescriber for PipxInstallAction.
// Returns "pipx install <package>".
func (a *PipxInstallAction) StatusMessage(params map[string]interface{}) string {
	pkg, ok := GetString(params, "package")
	if !ok || pkg == "" {
		return ""
	}
	pkg = progress.SanitizeDisplayString(pkg)
	if pkg == "" {
		return ""
	}
	return "pipx install " + pkg
}

// StatusMessage implements ActionDescriber for GemInstallAction.
// Returns "gem install <gem>".
func (a *GemInstallAction) StatusMessage(params map[string]interface{}) string {
	gem, ok := GetString(params, "gem")
	if !ok || gem == "" {
		return ""
	}
	gem = progress.SanitizeDisplayString(gem)
	if gem == "" {
		return ""
	}
	return "gem install " + gem
}
