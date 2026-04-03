package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that the tsuku environment is configured correctly",
	Long: `Verify that the tsuku environment is healthy: home directory exists,
tools/current is in PATH, and state file is accessible.

Exits with a non-zero status if any check fails, making it suitable
for use as a gate in scripts and CI:

  tsuku doctor || exit 1`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("failed to get config: %w", err)
		}

		homeDir, err := filepath.Abs(cfg.HomeDir)
		if err != nil {
			return fmt.Errorf("failed to resolve home directory: %w", err)
		}

		fmt.Println("Checking tsuku environment...")
		failed := false

		// Check 1: Home directory
		fmt.Fprintf(os.Stdout, "  Home directory: %s", homeDir)
		if info, err := os.Stat(homeDir); err != nil {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    Directory does not exist\n")
			fmt.Fprintf(os.Stderr, "    Run: tsuku install <tool> to create it\n")
			failed = true
		} else if !info.IsDir() {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    Path exists but is not a directory\n")
			failed = true
		} else {
			fmt.Println(" ... ok")
		}

		// Check 2: tools/current in PATH
		currentDir := filepath.Join(homeDir, "tools", "current")
		fmt.Fprintf(os.Stdout, "  tools/current in PATH")
		pathDirs := filepath.SplitList(os.Getenv("PATH"))
		found := false
		for _, dir := range pathDirs {
			absDir, _ := filepath.Abs(dir)
			if absDir == currentDir {
				found = true
				break
			}
		}
		if found {
			fmt.Println(" ... ok")
		} else {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    %s is not in your PATH\n", currentDir)
			fmt.Fprintf(os.Stderr, "    Run: eval $(tsuku shellenv)\n")
			failed = true
		}

		// Check 3: bin directory in PATH
		binDir := filepath.Join(homeDir, "bin")
		fmt.Fprintf(os.Stdout, "  bin in PATH")
		foundBin := false
		for _, dir := range pathDirs {
			absDir, _ := filepath.Abs(dir)
			if absDir == binDir {
				foundBin = true
				break
			}
		}
		if foundBin {
			fmt.Println(" ... ok")
		} else {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    %s is not in your PATH\n", binDir)
			fmt.Fprintf(os.Stderr, "    Run: eval $(tsuku shellenv)\n")
			failed = true
		}

		// Check 4: State file
		stateFile := filepath.Join(homeDir, "state.json")
		fmt.Fprintf(os.Stdout, "  State file")
		if _, err := os.Stat(stateFile); err != nil {
			if os.IsNotExist(err) {
				fmt.Println(" ... ok (no tools installed yet)")
			} else {
				fmt.Println(" ... FAIL")
				fmt.Fprintf(os.Stderr, "    Cannot access state file: %v\n", err)
				failed = true
			}
		} else {
			fmt.Println(" ... ok")
		}

		// Check 5: Shell.d health
		fmt.Fprintf(os.Stdout, "  Shell integration")

		// Collect content hashes from cleanup actions in state
		var contentHashes map[string]string
		stateMgr := install.NewStateManager(cfg)
		state, stateErr := stateMgr.Load()
		if stateErr == nil && state != nil {
			contentHashes = make(map[string]string)
			for _, ts := range state.Installed {
				activeVer := ts.ActiveVersion
				if activeVer == "" {
					activeVer = ts.Version // legacy
				}
				if vs, ok := ts.Versions[activeVer]; ok {
					for _, ca := range vs.CleanupActions {
						if ca.ContentHash != "" {
							contentHashes[ca.Path] = ca.ContentHash
						}
					}
				}
			}
		}

		shellCheck := shellenv.CheckShellD(homeDir, contentHashes)

		// Count total active scripts across all shells
		totalScripts := 0
		for _, scripts := range shellCheck.ActiveScripts {
			totalScripts += len(scripts)
		}

		if totalScripts == 0 {
			fmt.Println(" ... ok (no shell hooks)")
		} else if !shellCheck.HasIssues() {
			// Build a summary of active shells
			var shellSummary []string
			for shell, scripts := range shellCheck.ActiveScripts {
				shellSummary = append(shellSummary, fmt.Sprintf("%d %s", len(scripts), shell))
			}
			fmt.Printf(" ... ok (%s)\n", strings.Join(shellSummary, ", "))
		} else {
			fmt.Println(" ... FAIL")
			failed = true

			for shell, stale := range shellCheck.CacheStale {
				if stale {
					fmt.Fprintf(os.Stderr, "    %s cache is stale (run: tsuku doctor --rebuild-cache)\n", shell)
				}
			}
			for _, name := range shellCheck.HashMismatches {
				fmt.Fprintf(os.Stderr, "    %s: content hash mismatch\n", name)
			}
			for _, name := range shellCheck.Symlinks {
				fmt.Fprintf(os.Stderr, "    %s: symlink detected (security risk)\n", name)
			}
			for _, se := range shellCheck.SyntaxErrors {
				fmt.Fprintf(os.Stderr, "    %s: syntax error: %s\n", se.File, se.Message)
			}
		}

		// Check 6: Orphaned staging directories
		fmt.Fprintf(os.Stdout, "  Orphaned staging dirs")
		toolsDir := filepath.Join(homeDir, "tools")
		var orphanedStaging []string
		if entries, err := os.ReadDir(toolsDir); err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".staging-") {
					orphanedStaging = append(orphanedStaging, e.Name())
				}
			}
		}
		if len(orphanedStaging) == 0 {
			fmt.Println(" ... ok")
		} else {
			fmt.Printf(" ... WARN (%d found)\n", len(orphanedStaging))
			for _, name := range orphanedStaging {
				fmt.Fprintf(os.Stderr, "    %s (remove manually: rm -rf %s)\n", name, filepath.Join(toolsDir, name))
			}
		}

		// Check 7: Stale notices
		fmt.Fprintf(os.Stdout, "  Stale notices")
		noticesDir := notices.NoticesDir(homeDir)
		var staleNotices []string
		staleThreshold := 30 * 24 * time.Hour
		if entries, err := os.ReadDir(noticesDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				if time.Since(info.ModTime()) > staleThreshold {
					staleNotices = append(staleNotices, e.Name())
				}
			}
		}
		if len(staleNotices) == 0 {
			fmt.Println(" ... ok")
		} else {
			fmt.Printf(" ... WARN (%d stale, >30 days old)\n", len(staleNotices))
			fmt.Fprintf(os.Stderr, "    Run: rm %s/*.json to clear\n", noticesDir)
		}

		// Summary
		if failed {
			fmt.Println()
			return fmt.Errorf("environment check failed")
		}

		fmt.Println()
		fmt.Println("Everything looks good!")
		return nil
	},
}
