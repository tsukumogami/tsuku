package main

import (
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

// TestAutoApplyInstaller_ShellInitChangeReachesTheNoticeInbox is the auto-apply
// half of issue #2470, and the reason the warning is worth emitting on a path
// that writes to /dev/null.
//
// apply-updates redirects stdout and stderr before it does anything, so nothing
// printed there can reach a user. The InboxReporter is the channel that does:
// warnings accumulate in memory and land as a notice the next foreground command
// displays. This drives the real callback with the install substituted and reads
// the notice back off disk.
func TestAutoApplyInstaller_ShellInitChangeReachesTheNoticeInbox(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)
	noticesDir := notices.NoticesDir(cfg.HomeDir)

	recordUpdateVersion(t, mgr, "mytool", "1.0.0", []install.CleanupAction{
		updateFragment("mytool", "1.0.0", "bash", "before"),
	})

	installFn, flushNotices := autoApplyInstaller(mgr, noticesDir, func(installArgs) error {
		// What the install would leave behind: a new active version whose bash
		// init has different content.
		recordUpdateVersion(t, mgr, "mytool", "2.0.0", []install.CleanupAction{
			updateFragment("mytool", "2.0.0", "bash", "after"),
		})
		return nil
	})

	if err := installFn("mytool", "2.0.0", ""); err != nil {
		t.Fatalf("the auto-apply install callback failed: %v", err)
	}
	flushNotices()

	all, err := notices.ReadAllNotices(noticesDir)
	if err != nil {
		t.Fatalf("ReadAllNotices() error = %v", err)
	}

	var found bool
	for _, n := range all {
		for _, msg := range n.Messages {
			if strings.Contains(msg, "shell init changed for mytool (bash)") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("the shell init change never reached the notice inbox; notices = %+v", all)
	}
}

// A background update that changed nothing about the tool's shell init must not
// manufacture a notice. An inbox that fills up on every routine update is one
// the user stops reading.
func TestAutoApplyInstaller_UnchangedShellInitWritesNoNotice(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)
	noticesDir := notices.NoticesDir(cfg.HomeDir)

	recordUpdateVersion(t, mgr, "mytool", "1.0.0", []install.CleanupAction{
		updateFragment("mytool", "1.0.0", "bash", "same"),
	})

	installFn, flushNotices := autoApplyInstaller(mgr, noticesDir, func(installArgs) error {
		recordUpdateVersion(t, mgr, "mytool", "2.0.0", []install.CleanupAction{
			updateFragment("mytool", "2.0.0", "bash", "same"),
		})
		return nil
	})

	if err := installFn("mytool", "2.0.0", ""); err != nil {
		t.Fatalf("the auto-apply install callback failed: %v", err)
	}
	flushNotices()

	all, err := notices.ReadAllNotices(noticesDir)
	if err != nil {
		t.Fatalf("ReadAllNotices() error = %v", err)
	}
	for _, n := range all {
		for _, msg := range n.Messages {
			if strings.Contains(msg, "shell init changed") {
				t.Errorf("an unchanged shell init produced a notice: %q", msg)
			}
		}
	}
}
