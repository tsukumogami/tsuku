package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var (
	updateDryRun bool
	updateAll    bool
)

var updateCmd = &cobra.Command{
	Use:   "update [tool]",
	Short: "Update a tool to the latest version",
	Long: `Update an installed tool to its latest version within pin boundaries.

Use --all to update all installed tools at once.

Examples:
  tsuku update kubectl
  tsuku update terraform
  tsuku update --all
  tsuku update --all --dry-run`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if updateAll && len(args) > 0 {
			fmt.Fprintf(os.Stderr, "Error: --all and a tool name are mutually exclusive\n")
			exitWithCode(ExitUsage)
		}
		if !updateAll && len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: provide a tool name or use --all\n")
			_ = cmd.Usage()
			exitWithCode(ExitUsage)
		}

		if updateAll {
			runUpdateAll(cmd)
			return
		}

		toolName := args[0]

		// Initialize telemetry
		telemetryClient := telemetry.NewClient()
		telemetry.ShowNoticeIfNeeded()

		// Check if installed
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		previousVersion, installed := activeVersionOf(tools, toolName)

		if !installed {
			fmt.Fprintf(os.Stderr, "Error: %s is not installed. Use 'tsuku install %s' to install it.\n", toolName, toolName)
			exitWithCode(ExitGeneral)
		}

		// For distributed sources, use GetFromSource to fetch the recipe
		// directly from the recorded provider. This avoids recipe shadowing
		// where a local or central recipe with the same name would take
		// priority in the chain. The recipe is cached in the loader so
		// runInstall can find it.
		state, _ := mgr.GetState().Load()
		if r, err := loadRecipeForTool(context.Background(), toolName, state, cfg); err == nil && r != nil {
			loader.CacheRecipe(toolName, r)
		}

		if updateDryRun {
			printInfof("Checking updates for %s...\n", toolName)
			if err := runDryRun(toolName, ""); err != nil {
				printError(err)
				exitWithCode(ExitInstallFailed)
			}
			return
		}

		// Read the Requested field to respect install-time version constraint.
		// This ensures "tsuku update node" after "tsuku install node@18" stays
		// within 18.x.y instead of jumping to the absolute latest.
		var reqVersion string
		if state != nil {
			if ts, ok := state.Installed[toolName]; ok {
				if vs, ok := ts.Versions[ts.ActiveVersion]; ok {
					reqVersion = vs.Requested
				}
			}
		}

		// Create a reporter here so that both the install progress and the
		// outcome line share the same stream (stderr). This lets the TTY
		// spinner be replaced in-place by the permanent outcome line via
		// reporter.Log, rather than the spinner being cleared on stderr and
		// the outcome appearing on a separate stdout line. See #2280/#2359.
		reporter := progress.NewTTYReporter(os.Stderr)

		newVersion, err := updateInstalledTool(mgr, toolName, reporter.Warn, func() error {
			return runInstallWithReporter(installevents.WithSource(globalCtx, installevents.SourceManual), installArgs{
				Tool:            toolName,
				ReqVersion:      reqVersion,
				IsExplicit:      true,
				TelemetryClient: telemetryClient,
				Reporter:        reporter,
			})
		})
		if err != nil {
			reporter.Stop()
			// UpdateFailed event was already published from Manager.Install
			// via the bus; the telemetry subscriber emitted the
			// UpdateOutcomeFailure event. No direct telemetry call needed.
			exitWithCode(ExitInstallFailed)
		}

		// Emit the no-op outcome via the reporter so the TTY spinner is
		// replaced in-place. The update outcome (new version) is already
		// handled by the install reporter's "✅ <name>@<version>" line.
		if msg := updateOutcomeMessage(toolName, previousVersion, newVersion); msg != "" && !quietFlag {
			reporter.Log("%s", msg)
		}
		reporter.Stop()
		reporter.FlushDeferred()

		// Send action telemetry for update. The UpdateOutcomeSuccess
		// event is emitted automatically by the telemetry subscriber when
		// Manager.Install publishes Updated via the bus; the notices
		// subscriber writes the success notice. The action event below is
		// distinct from outcome telemetry and stays.
		if telemetryClient != nil && newVersion != "" {
			event := telemetry.NewUpdateEvent(toolName, previousVersion, newVersion)
			telemetryClient.Send(event)
		}
	},
}

// updateOutcomeMessage returns the user-facing summary line for an
// update operation, or the empty string when no extra line is needed.
//
// The install machinery's reporter already prints a permanent
// "✅ <name>@<version>" line per successful install (see #2280), so
// printing an "Updated X: A -> B" line here would duplicate it. We
// only emit a line for the no-op case, where the install's
// "is already installed" message is a transient TTY spinner that
// gets cleared on command exit.
//
// Branches:
//
//	newVersion == ""              → "" (no message; defensive)
//	newVersion == previousVersion → "<tool> is already at the latest version (<v>)."
//	newVersion != previousVersion → "" (success line already printed by install reporter)
func updateOutcomeMessage(toolName, previousVersion, newVersion string) string {
	if newVersion == "" || newVersion != previousVersion {
		return ""
	}
	return fmt.Sprintf("%s is already at the latest version (%s).", toolName, newVersion)
}

// updateWithCleanup runs an install that replaces one version of a tool with
// another, bracketed by the lifecycle-aware cleanup pass every such install
// needs: snapshot what the outgoing version wrote outside its own directory,
// install, then reconcile against what the incoming version writes.
//
// It exists so the three paths that update a tool -- `tsuku update <tool>`,
// `tsuku update --all`, and background auto-apply -- cannot each hold a
// different two thirds of that sequence. They already had: only the single-tool
// path ran the pass at all, which is issue #2470.
//
// The install error is returned unchanged, and nothing is reconciled when the
// install fails.
func updateWithCleanup(mgr *install.Manager, toolName string, warn install.WarnFunc, doInstall func() error) error {
	snapshot := mgr.SnapshotCleanup(toolName)
	if err := doInstall(); err != nil {
		return err
	}
	mgr.ReconcileUpdate(snapshot, warn)
	return nil
}

// updateInstalledTool is one foreground tool update end to end: the
// cleanup-bracketed install, then the version that ended up active. Both
// `tsuku update <tool>` and `tsuku update --all` go through it, so neither the
// pass nor the version readback can be right on one and wrong on the other.
//
// The returned version is empty when the install failed, and when state cannot
// say which version is active afterwards. Callers that treat "no version" as
// "unchanged" have to say so themselves -- the two commands differ on that.
//
// doInstall is a parameter rather than a direct call so a test can drive the
// sequence without resolving a recipe or downloading anything.
func updateInstalledTool(mgr *install.Manager, toolName string, warn install.WarnFunc, doInstall func() error) (string, error) {
	if err := updateWithCleanup(mgr, toolName, warn, doInstall); err != nil {
		return "", err
	}
	return activeVersionAfterUpdate(mgr, toolName), nil
}

// activeVersionAfterUpdate reports which version of the tool is active now.
//
// List returns one entry per retained version, so the entry flagged IsActive is
// the one to read: an older retained version would otherwise shadow the version
// that was just installed.
func activeVersionAfterUpdate(mgr *install.Manager, toolName string) string {
	tools, err := mgr.List()
	if err != nil {
		return ""
	}
	version, _ := activeVersionOf(tools, toolName)
	return version
}

// activeVersionOf reports the active version of toolName among tools, and
// whether tools holds any entry for it at all.
//
// The two answers are separate because List returns one entry per retained
// version: a tool can be present under several versions while only one of them
// is active, and it can in principle be present with none of them flagged
// active. Callers deciding "is this installed?" need the second return; callers
// comparing versions across an update need the first.
//
// Reading the first name match instead of the active entry is the bug this
// exists to prevent. List sorts ascending by version, so the first match is the
// *oldest* retained version -- for a tool sitting at 0.19.1 with 0.16.0 still on
// disk, it reads 0.16.0. Compared against the version active after an update
// that changed nothing, the two never match: `tsuku update` printed no
// "already at the latest version" line and sent a telemetry event claiming an
// upgrade from 0.16.0 to 0.19.1 that never happened.
func activeVersionOf(tools []install.InstalledTool, toolName string) (string, bool) {
	installed := false
	for _, tool := range tools {
		if tool.Name != toolName {
			continue
		}
		installed = true
		if tool.IsActive {
			return tool.Version, true
		}
	}
	return "", installed
}

// isDistributedSource returns true if the source string is a distributed
// "owner/repo" source (as opposed to "central", "local", or "embedded").
func isDistributedSource(source string) bool {
	return strings.Contains(source, "/")
}

// runUpdateAll updates all installed tools within their pin boundaries.
func runUpdateAll(cmd *cobra.Command) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	mgr := install.New(cfg)
	tools, err := mgr.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if len(tools) == 0 {
		printInfo("No tools installed.")
		return
	}

	state, _ := mgr.GetState().Load()
	telemetryClient := telemetry.NewClient()
	telemetry.ShowNoticeIfNeeded()

	var updated, upToDate, failed, skipped int

	for _, tool := range tools {
		// mgr.List returns one entry per retained version. Update each
		// tool by name only once; iterate the active version's entry.
		if !tool.IsActive {
			continue
		}

		// Read pin constraint
		var requested string
		if state != nil {
			if ts, ok := state.Installed[tool.Name]; ok {
				if vs, ok := ts.Versions[ts.ActiveVersion]; ok {
					requested = vs.Requested
				}
			}
		}

		// Skip exact-pinned tools
		if install.PinLevelFromRequested(requested) == install.PinExact {
			skipped++
			continue
		}

		if updateDryRun {
			printInfof("Checking %s...\n", tool.Name)
			if err := runDryRun(tool.Name, ""); err != nil {
				printError(err)
				failed++
			}
			continue
		}

		// Load recipe from correct source
		if r, loadErr := loadRecipeForTool(context.Background(), tool.Name, state, cfg); loadErr == nil && r != nil {
			loader.CacheRecipe(tool.Name, r)
		}

		previousVersion := tool.Version

		// Per-tool reporter so the spinner is replaced in-place by the
		// outcome line, consistent with the single-tool path.
		toolReporter := progress.NewTTYReporter(os.Stderr)

		newVersion, err := updateInstalledTool(mgr, tool.Name, toolReporter.Warn, func() error {
			return runInstallWithReporter(installevents.WithSource(globalCtx, installevents.SourceManual), installArgs{
				Tool:            tool.Name,
				ReqVersion:      requested,
				IsExplicit:      true,
				TelemetryClient: telemetryClient,
				Reporter:        toolReporter,
			})
		})
		if err != nil {
			toolReporter.Log("failed to update %s: %v", tool.Name, err)
			toolReporter.Stop()
			// UpdateFailed was published from Manager.Install; the
			// telemetry subscriber emitted the outcome event.
			failed++
			continue
		}

		// Whether the install changed the active version or the tool was
		// already at latest decides which counter moves. The install
		// machinery's "is already installed" status is a transient TTY
		// message, so up-to-date has to be counted separately for the summary
		// to be accurate.
		//
		// When state cannot say which version is active, count the tool as
		// unchanged rather than as an update. Reporting an update that may not
		// have happened is the worse of the two errors here.
		if newVersion == "" {
			newVersion = previousVersion
		}

		// Emit the no-op outcome via the reporter so the TTY spinner is
		// replaced in-place. Real updates already get a permanent
		// "✅ <name>@<version>" line from the install reporter (#2280).
		if newVersion == previousVersion {
			if !quietFlag {
				toolReporter.Log("%s is already at the latest version (%s).", tool.Name, newVersion)
			}
			upToDate++
		} else {
			updated++
		}

		// The notices subscriber already wrote the success notice via
		// Manager.Install's published Updated event. No direct
		// WriteNotice / RemoveNotice calls needed here.

		toolReporter.Stop()
		toolReporter.FlushDeferred()
	}

	if updateDryRun {
		return
	}

	total := updated + upToDate + failed
	if total == 0 {
		if skipped > 0 {
			printInfo("All tools are exact-pinned, nothing to update.")
		} else {
			printInfo("No tools to update.")
		}
		return
	}

	switch {
	case failed == 0 && updated == 0:
		printInfo("All tools are up to date.")
	case failed == 0:
		printInfof("Updated %d, up to date %d.\n", updated, upToDate)
	default:
		printInfof("Updated %d, up to date %d, failed %d.\n", updated, upToDate, failed)
	}
}

func init() {
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated without making changes")
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all installed tools within pin boundaries")
}
