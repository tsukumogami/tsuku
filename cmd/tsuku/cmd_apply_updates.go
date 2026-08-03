package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/updates"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var applyUpdatesCmd = &cobra.Command{
	Use:           "apply-updates",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Redirect stdout/stderr to devnull for truly silent background operation
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		cfg, err := config.DefaultConfig()
		if err != nil {
			return nil
		}

		userCfg, err := userconfig.Load()
		if err != nil {
			return nil
		}

		// Background auto-apply: tag every event with SourceAuto on the
		// context once so all downstream callsites read it via
		// installevents.SourceFromContext.
		ctx := installevents.WithSource(globalCtx, installevents.SourceAuto)

		installFn, flushNotices := autoApplyInstaller(
			install.New(cfg),
			notices.NoticesDir(cfg.HomeDir),
			func(args installArgs) error { return runInstallWithReporter(ctx, args) },
		)

		updates.MaybeAutoApply(cfg, userCfg, nil, installFn, nil)
		flushNotices()

		return nil
	},
}

// autoApplyInstaller builds the install callback the background apply path hands
// to MaybeAutoApply, plus the function that flushes what those installs had to
// say into $TSUKU_HOME/notices/.
//
// It takes the install itself as a parameter, rather than calling
// runInstallWithReporter directly, so the wiring below can be driven by a test
// that has no network.
//
// The stale-cleanup pass belongs here rather than inside MaybeAutoApply: that
// function decides which tools to update, and this callback decides how one gets
// installed. Bracketing it here gives the background path the same pass the two
// foreground ones run, through the same helper.
//
// Warnings do reach the user even though apply-updates sends stdout and stderr
// to /dev/null, because an InboxReporter accumulates them in memory and writes
// them as a notice on Stop(). That is what makes the ordering load-bearing: the
// reconcile has to happen inside the callback, before flushNotices runs, or the
// warning is composed after the only thing that would have persisted it.
func autoApplyInstaller(
	mgr *install.Manager,
	noticesDir string,
	runInstall func(installArgs) error,
) (updates.InstallFunc, func()) {
	var reporters []*progress.InboxReporter

	installFn := func(toolName, version, constraint string) error {
		reporter := progress.NewInboxReporter(toolName, noticesDir)
		reporters = append(reporters, reporter)

		return updateWithCleanup(mgr, toolName, reporter.Warn, func() error {
			return runInstall(installArgs{
				Tool:              toolName,
				ReqVersion:        version,
				VersionConstraint: constraint,
				Reporter:          reporter,
			})
		})
	}

	// Stopping the reporters is deferred until MaybeAutoApply has written its
	// success notices. A reporter with nothing accumulated returns early and
	// leaves that notice intact; one with warnings overwrites it with a richer
	// notice.
	return installFn, func() {
		for _, r := range reporters {
			r.Stop()
		}
	}
}

func init() {
	rootCmd.AddCommand(applyUpdatesCmd)
}
