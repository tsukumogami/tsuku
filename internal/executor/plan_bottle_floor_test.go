package executor

import (
	"errors"
	"strings"
	"testing"
)

// darwinBottlePlan builds a darwin plan whose Homebrew steps record the
// given bottle tags: mainTag on the tool itself and depTag on a nested
// dependency. An empty tag leaves the param out, as plans written before
// tags were recorded do.
func darwinBottlePlan(mainTag, depTag string) *InstallationPlan {
	relocate := func(formula, tag string) ResolvedStep {
		params := map[string]interface{}{"formula": formula}
		if tag != "" {
			params["bottle_tag"] = tag
		}
		return ResolvedStep{Action: "homebrew_relocate", Params: params}
	}
	return &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "curl",
		Version:       "8.18.0",
		Platform:      Platform{OS: "darwin", Arch: "arm64"},
		Steps:         []ResolvedStep{relocate("curl", mainTag)},
		Dependencies: []DependencyPlan{{
			Tool:  "libssh2",
			Steps: []ResolvedStep{relocate("libssh2", depTag)},
			Dependencies: []DependencyPlan{{
				Tool:  "openssl",
				Steps: []ResolvedStep{relocate("openssl@3", depTag)},
			}},
		}},
	}
}

// bottleFloorMessages returns the validation messages about bottles built
// for a newer macOS. Other errors (such as the plan being for darwin while
// the test runs on Linux) are left out.
func bottleFloorMessages(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var pve *PlanValidationError
	if !errors.As(err, &pve) {
		t.Fatalf("unexpected error type %T: %v", err, err)
	}
	var msgs []string
	for _, e := range pve.Errors {
		if strings.Contains(e.Message, "is built for macOS") || strings.Contains(e.Message, "doesn't know") {
			msgs = append(msgs, e.Message)
		}
	}
	return msgs
}

func TestValidatePlan_RefusesBottleNewerThanHost(t *testing.T) {
	old := hostMacOSVersion
	defer func() { hostMacOSVersion = old }()

	cases := []struct {
		name     string
		host     int
		mainTag  string
		depTag   string
		wantMsgs []string
	}{
		{
			name: "Tahoe bottles on a Sequoia host", host: 15,
			mainTag: "arm64_tahoe", depTag: "arm64_tahoe",
			wantMsgs: []string{
				"bottle arm64_tahoe is built for macOS 26, but this system runs macOS 15",
				"dependency libssh2 (of curl): bottle arm64_tahoe",
				"dependency openssl (of libssh2): bottle arm64_tahoe",
			},
		},
		{
			name: "Sequoia bottle on a Sonoma host", host: 14,
			mainTag: "arm64_sequoia", depTag: "arm64_sonoma",
			wantMsgs: []string{"bottle arm64_sequoia is built for macOS 15, but this system runs macOS 14"},
		},
		{name: "older bottles on a newer host", host: 26, mainTag: "arm64_sequoia", depTag: "arm64_sonoma"},
		{name: "same release", host: 15, mainTag: "arm64_sequoia", depTag: "arm64_sequoia"},
		{name: "plan written before tags were recorded", host: 14},
		{name: "host version unknown", host: 0, mainTag: "arm64_tahoe", depTag: "arm64_tahoe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostMacOSVersion = func() int { return tc.host }
			msgs := bottleFloorMessages(t, ValidatePlan(darwinBottlePlan(tc.mainTag, tc.depTag)))
			if len(msgs) != len(tc.wantMsgs) {
				t.Fatalf("got %d bottle errors %q, want %d", len(msgs), msgs, len(tc.wantMsgs))
			}
			for i, want := range tc.wantMsgs {
				if !strings.Contains(msgs[i], want) {
					t.Errorf("error %d = %q, want it to contain %q", i, msgs[i], want)
				}
			}
		})
	}
}

func TestValidatePlan_BottleFloorOnlyForDarwinPlans(t *testing.T) {
	old := hostMacOSVersion
	defer func() { hostMacOSVersion = old }()
	hostMacOSVersion = func() int { return 15 }

	plan := darwinBottlePlan("arm64_tahoe", "")
	plan.Platform = Platform{OS: "linux", Arch: "amd64"}
	if msgs := bottleFloorMessages(t, ValidatePlan(plan)); len(msgs) != 0 {
		t.Errorf("linux plan got bottle errors %q", msgs)
	}
}
