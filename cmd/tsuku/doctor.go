package main

import (
	"bytes"
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

var doctorFixFlag bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that the tsuku environment is configured correctly",
	Long: `Verify that the tsuku environment is healthy: home directory exists,
tools/current is in PATH, and state file is accessible.

Exits with a non-zero status if any check fails, making it suitable
for use as a gate in scripts and CI:

  tsuku doctor || exit 1

Use --fix to automatically repair stale env files and shell caches.`,
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

		failed, selection := runDoctorChecks(cfg, homeDir)

		if doctorFixFlag {
			// Apply repairs
			fixed := false

			// Fix stale env file
			envData, envErr := os.ReadFile(cfg.EnvFile())
			envStale := envErr != nil || !bytes.Equal(envData, []byte(config.EnvFileContent))
			if envStale {
				if fixErr := cfg.EnsureEnvFile(); fixErr != nil {
					fmt.Fprintf(os.Stderr, "  Failed to repair env file: %v\n", fixErr)
				} else {
					fmt.Println("  Repaired: env file written")
					fixed = true
				}
			}

			// Fix stale shell caches
			shellCheck := shellenv.CheckShellD(homeDir, selection)
			for shell, stale := range shellCheck.CacheStale {
				if stale {
					if fixErr := shellenv.RebuildShellCache(homeDir, shell, selection); fixErr != nil {
						fmt.Fprintf(os.Stderr, "  Failed to rebuild %s cache: %v\n", shell, fixErr)
					} else {
						fmt.Printf("  Repaired: %s shell cache rebuilt\n", shell)
						fixed = true
					}
				}
			}

			if fixed {
				fmt.Println()
				fmt.Println("Re-checking after repair...")
				failed, _ = runDoctorChecks(cfg, homeDir)
			}
		}

		if failed {
			fmt.Println()
			return fmt.Errorf("environment check failed")
		}

		fmt.Println()
		fmt.Println("Everything looks good!")
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFixFlag, "fix", false, "Repair stale env file and shell caches")
}

// runDoctorChecks performs all environment checks and returns whether any check
// failed and the shell.d selection loaded from state (needed for cache repair).
func runDoctorChecks(cfg *config.Config, homeDir string) (failed bool, selection shellenv.ShellDSelection) {
	fmt.Println("Checking tsuku environment...")

	// Set by check 6, which is where state is loaded, and consumed by check 9.
	var managedBinaries []shellenv.ManagedBinaries

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

	// Check 5: Env file
	fmt.Fprintf(os.Stdout, "  Env file")
	envData, envErr := os.ReadFile(cfg.EnvFile())
	if envErr != nil {
		fmt.Println(" ... FAIL")
		if os.IsNotExist(envErr) {
			fmt.Fprintf(os.Stderr, "    Env file missing: %s\n", cfg.EnvFile())
		} else {
			fmt.Fprintf(os.Stderr, "    Cannot read env file: %v\n", envErr)
		}
		fmt.Fprintf(os.Stderr, "    Run: tsuku doctor --fix\n")
		failed = true
	} else if !bytes.Equal(envData, []byte(config.EnvFileContent)) {
		fmt.Println(" ... FAIL")
		fmt.Fprintf(os.Stderr, "    Env file is stale: %s\n", cfg.EnvFile())
		fmt.Fprintf(os.Stderr, "    Run: tsuku doctor --fix\n")
		failed = true
	} else {
		fmt.Println(" ... ok")
	}

	// Check 6: Shell.d health
	fmt.Fprintf(os.Stdout, "  Shell integration")

	// Project state onto shell.d: which recorded fragments belong to an active
	// version, and which belong to a version that is installed but not active.
	stateMgr := install.NewStateManager(cfg)
	state, stateErr := stateMgr.Load()
	if stateErr == nil {
		selection = install.BuildShellDSelection(state)
		managedBinaries = install.BuildManagedBinaries(state)
	}

	shellCheck := shellenv.CheckShellD(homeDir, selection)

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
				fmt.Fprintf(os.Stderr, "    %s cache is stale (run: tsuku doctor --fix)\n", shell)
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

	// Check 7: Orphaned staging directories
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

	// Check 8: Stale notices
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

	// Check 9: Tool precedence.
	//
	// Checks 2 and 3 ask whether tsuku's directories are on PATH. This one asks
	// whether they win. A competing installer that prepends its own prefix after
	// $TSUKU_HOME/env has been sourced leaves both checks above green while every
	// command actually runs the other copy.
	//
	// It warns rather than fails. Doctor is documented as a CI gate, and which
	// binary wins is a property of the user's PATH that they may well have
	// arranged deliberately.
	fmt.Fprintf(os.Stdout, "  Tool precedence")
	shadowed := shellenv.CheckPrecedence(shellenv.PrecedenceInput{
		TsukuHome: homeDir,
		PathDirs:  pathDirs,
		Tools:     managedBinaries,
	})
	if len(shadowed) == 0 {
		fmt.Println(" ... ok")
	} else {
		fmt.Printf(" ... WARN (%d shadowed)\n", len(shadowed))
		for _, s := range shadowed {
			// Name the tool too when it differs from the binary, so the reader
			// knows which `tsuku` entry to act on -- ripgrep provides rg.
			label := s.Binary
			if s.Tool != s.Binary {
				label = fmt.Sprintf("%s (%s)", s.Binary, s.Tool)
			}
			fmt.Fprintf(os.Stderr, "    %s resolves to %s, not tsuku's %s\n", label, s.Resolved, s.Expected)
		}
		fmt.Fprintf(os.Stderr, "    Something earlier on PATH is shadowing a tsuku-managed tool.\n")
		fmt.Fprintf(os.Stderr, "    Move the tsuku entries ahead of it, or remove the other copy.\n")
	}

	return failed, selection
}
