package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/datamigration"
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

			// Retry a data migration the install could not finish. Only in the
			// stranded case: when the fragment still names an old location, the data
			// is where the user's shell expects it and moving it would break a
			// working install.
			if nvmDataRootInstalled(cfg) && nvmDataRootState(cfg, homeDir) == nvmDataRootStranded {
				result, mErr := datamigration.MigrateNvm(homeDir)
				switch {
				case mErr != nil:
					fmt.Fprintf(os.Stderr, "  Failed to move nvm data: %v\n", mErr)
				case len(result.Moved) > 0:
					fmt.Printf("  Repaired: moved %d nvm item(s) to %s\n", len(result.Moved), datamigration.NvmDataDir(homeDir))
					fixed = true
				}
				// Conflicts and cross-device failures reproduce on every retry, so say
				// so rather than reporting a repair that did not happen.
				if msg := datamigration.ConflictReport(datamigration.NvmDataDir(homeDir), result.Conflicts); msg != "" {
					fmt.Fprintln(os.Stderr, "  "+msg)
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

	// Check 9: nvm data root.
	//
	// The only tool-specific check here, and gated so it prints nothing for anyone who
	// does not have nvm installed. Kept as one hunk so it comes out in one cut once the
	// affected releases have aged out.
	if nvmDataRootInstalled(cfg) {
		fmt.Fprintf(os.Stdout, "  nvm data root")
		switch state := nvmDataRootState(cfg, homeDir); state {
		case nvmDataRootOK:
			fmt.Println(" ... ok")
		case nvmDataRootStranded:
			// The export already names the new root, so the user's shell is pointing at
			// an empty directory right now.
			fmt.Println(" ... FAIL (data left behind in an old location)")
			fmt.Fprintf(os.Stderr, "    Your Node versions are not in %s\n", datamigration.NvmDataDir(homeDir))
			fmt.Fprintln(os.Stderr, "    Run: tsuku doctor --fix")
			failed = true
		case nvmDataRootLegacy:
			// Nothing is broken: the fragment and the data agree. Say where the data is
			// and leave it alone -- moving it now would break a working install.
			fmt.Println(" ... WARN (using a legacy location)")
			fmt.Fprintln(os.Stderr, "    Your Node versions are still inside a tsuku-managed tool directory.")
			fmt.Fprintf(os.Stderr, "    They move to %s the next time nvm updates.\n", datamigration.NvmDataDir(homeDir))
		}
	}

	return failed, selection
}

// nvmDataRoot states describe how nvm's data and its exported root relate.
const (
	// nvmDataRootOK means nothing is in a legacy location.
	nvmDataRootOK = iota
	// nvmDataRootStranded means the export names the new root but the data did not
	// arrive -- the shell is pointing somewhere empty.
	nvmDataRootStranded
	// nvmDataRootLegacy means the export still names an old location where the data
	// actually is. Working, and not ours to move until nvm next installs.
	nvmDataRootLegacy
)

// nvmDataRootInstalled reports whether nvm is installed through tsuku.
func nvmDataRootInstalled(cfg *config.Config) bool {
	matches, _ := filepath.Glob(filepath.Join(cfg.ToolsDir, datamigration.NvmTool+"-*"))
	return len(matches) > 0
}

// nvmDataRootState classifies nvm's data against what the user's shell is told.
//
// The active shell fragment is read rather than inferred from what is on disk. A
// stat of the data root would get this wrong after a rollback: activate re-selects
// fragments without re-running post-install, so the new root can be populated while the
// active fragment still names an old path.
func nvmDataRootState(cfg *config.Config, homeDir string) int {
	if len(datamigration.FindNvmSources(homeDir)) == 0 {
		return nvmDataRootOK
	}
	if exportedNvmDirIsDataRoot(cfg, homeDir) {
		return nvmDataRootStranded
	}
	return nvmDataRootLegacy
}

// exportedNvmDirIsDataRoot reports whether the shell fragment tsuku currently serves
// exports NVM_DIR at the stable data root.
func exportedNvmDirIsDataRoot(cfg *config.Config, homeDir string) bool {
	shellDDir := filepath.Join(homeDir, "share", "shell.d")
	entries, err := os.ReadDir(shellDDir)
	if err != nil {
		return false
	}
	want := datamigration.NvmDataDir(homeDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), actions.EnvFilePrefix+datamigration.NvmTool) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(shellDDir, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), want) {
			return true
		}
	}
	return false
}
