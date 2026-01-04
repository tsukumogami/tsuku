package sandbox

import (
	"reflect"
	"testing"

	"github.com/tsukumogami/tsuku/internal/executor"
)

func TestExtractPackages_NilPlan(t *testing.T) {
	t.Parallel()

	packages := ExtractPackages(nil)

	if packages != nil {
		t.Errorf("ExtractPackages(nil) = %v, want nil", packages)
	}
}

func TestExtractPackages_EmptyPlan(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{},
	}

	packages := ExtractPackages(plan)

	if packages != nil {
		t.Errorf("ExtractPackages(empty) = %v, want nil", packages)
	}
}

func TestExtractPackages_NoSystemDeps(t *testing.T) {
	t.Parallel()

	// Binary installation plan - no system dependency actions
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "download", Params: map[string]interface{}{"url": "https://example.com/tool.tar.gz"}},
			{Action: "extract", Params: map[string]interface{}{}},
			{Action: "install_binaries", Params: map[string]interface{}{}},
		},
	}

	packages := ExtractPackages(plan)

	if packages != nil {
		t.Errorf("ExtractPackages(binary plan) = %v, want nil", packages)
	}
}

func TestExtractPackages_AptInstall(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker-ce", "containerd.io"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker-ce", "containerd.io"}
	if !reflect.DeepEqual(packages["apt"], expected) {
		t.Errorf("packages[apt] = %v, want %v", packages["apt"], expected)
	}
}

func TestExtractPackages_AptRepo(t *testing.T) {
	t.Parallel()

	// apt_repo signals system deps but doesn't add packages directly
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_repo",
				Params: map[string]interface{}{
					"url":        "https://download.docker.com/linux/ubuntu",
					"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
					"key_sha256": "1234567890abcdef",
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil for apt_repo, want non-nil map")
	}

	// apt_repo doesn't add packages to the apt key
	if len(packages["apt"]) != 0 {
		t.Errorf("packages[apt] = %v, want empty (apt_repo doesn't add packages)", packages["apt"])
	}
}

func TestExtractPackages_AptPPA(t *testing.T) {
	t.Parallel()

	// apt_ppa signals system deps but doesn't add packages directly
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_ppa",
				Params: map[string]interface{}{
					"ppa": "deadsnakes/ppa",
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil for apt_ppa, want non-nil map")
	}
}

func TestExtractPackages_BrewInstall(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "brew_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"curl", "jq"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"curl", "jq"}
	if !reflect.DeepEqual(packages["brew"], expected) {
		t.Errorf("packages[brew] = %v, want %v", packages["brew"], expected)
	}
}

func TestExtractPackages_BrewCask(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "brew_cask",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker"}
	if !reflect.DeepEqual(packages["brew"], expected) {
		t.Errorf("packages[brew] = %v, want %v", packages["brew"], expected)
	}
}

func TestExtractPackages_DnfInstall(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "dnf_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker", "podman"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker", "podman"}
	if !reflect.DeepEqual(packages["dnf"], expected) {
		t.Errorf("packages[dnf] = %v, want %v", packages["dnf"], expected)
	}
}

func TestExtractPackages_DnfRepo(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "dnf_repo",
				Params: map[string]interface{}{
					"url":        "https://download.docker.com/linux/fedora/docker-ce.repo",
					"key_url":    "https://download.docker.com/linux/fedora/gpg",
					"key_sha256": "1234567890abcdef",
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil for dnf_repo, want non-nil map")
	}

	// dnf_repo doesn't add packages to the dnf key
	if len(packages["dnf"]) != 0 {
		t.Errorf("packages[dnf] = %v, want empty (dnf_repo doesn't add packages)", packages["dnf"])
	}
}

func TestExtractPackages_PacmanInstall(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "pacman_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker", "docker-compose"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker", "docker-compose"}
	if !reflect.DeepEqual(packages["pacman"], expected) {
		t.Errorf("packages[pacman] = %v, want %v", packages["pacman"], expected)
	}
}

func TestExtractPackages_ApkInstall(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apk_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker", "docker-cli"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker", "docker-cli"}
	if !reflect.DeepEqual(packages["apk"], expected) {
		t.Errorf("packages[apk] = %v, want %v", packages["apk"], expected)
	}
}

func TestExtractPackages_ZypperInstall(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "zypper_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker"}
	if !reflect.DeepEqual(packages["zypper"], expected) {
		t.Errorf("packages[zypper] = %v, want %v", packages["zypper"], expected)
	}
}

func TestExtractPackages_MultipleManagers(t *testing.T) {
	t.Parallel()

	// A plan with multiple package managers (unlikely in practice,
	// but tests the aggregation logic)
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"curl"},
				},
			},
			{
				Action: "dnf_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"wget"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	if !reflect.DeepEqual(packages["apt"], []string{"curl"}) {
		t.Errorf("packages[apt] = %v, want [curl]", packages["apt"])
	}
	if !reflect.DeepEqual(packages["dnf"], []string{"wget"}) {
		t.Errorf("packages[dnf] = %v, want [wget]", packages["dnf"])
	}
}

func TestExtractPackages_AggregatesMultipleSteps(t *testing.T) {
	t.Parallel()

	// Multiple apt_install steps should be aggregated
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_repo",
				Params: map[string]interface{}{
					"url":        "https://download.docker.com/linux/ubuntu",
					"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
					"key_sha256": "1234567890abcdef",
				},
			},
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker-ce"},
				},
			},
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"containerd.io", "docker-ce-cli"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"docker-ce", "containerd.io", "docker-ce-cli"}
	if !reflect.DeepEqual(packages["apt"], expected) {
		t.Errorf("packages[apt] = %v, want %v", packages["apt"], expected)
	}
}

func TestExtractPackages_MissingPackagesParam(t *testing.T) {
	t.Parallel()

	// apt_install without packages param (edge case)
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_install",
				Params: map[string]interface{}{},
			},
		},
	}

	packages := ExtractPackages(plan)

	// Should return non-nil (has system deps) but apt key should be empty
	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want non-nil map (has system dep action)")
	}

	if len(packages["apt"]) != 0 {
		t.Errorf("packages[apt] = %v, want empty (no packages param)", packages["apt"])
	}
}

func TestExtractPackages_MixedWithNonSystemDeps(t *testing.T) {
	t.Parallel()

	// Plan with both system and non-system dependency actions
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "download", Params: map[string]interface{}{}},
			{Action: "extract", Params: map[string]interface{}{}},
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"build-essential"},
				},
			},
			{Action: "configure_make", Params: map[string]interface{}{}},
			{Action: "install_binaries", Params: map[string]interface{}{}},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"build-essential"}
	if !reflect.DeepEqual(packages["apt"], expected) {
		t.Errorf("packages[apt] = %v, want %v", packages["apt"], expected)
	}
}

func TestExtractPackages_BrewAggregation(t *testing.T) {
	t.Parallel()

	// Both brew_install and brew_cask should aggregate into "brew"
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "brew_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"wget"},
				},
			},
			{
				Action: "brew_cask",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker"},
				},
			},
		},
	}

	packages := ExtractPackages(plan)

	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"wget", "docker"}
	if !reflect.DeepEqual(packages["brew"], expected) {
		t.Errorf("packages[brew] = %v, want %v", packages["brew"], expected)
	}
}

func TestExtractSystemRequirements_NilPlan(t *testing.T) {
	t.Parallel()

	reqs := ExtractSystemRequirements(nil)

	if reqs != nil {
		t.Errorf("ExtractSystemRequirements(nil) = %v, want nil", reqs)
	}
}

func TestExtractSystemRequirements_EmptyPlan(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs != nil {
		t.Errorf("ExtractSystemRequirements(empty) = %v, want nil", reqs)
	}
}

func TestExtractSystemRequirements_AptRepoWithGPG(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_repo",
				Params: map[string]interface{}{
					"url":        "https://download.docker.com/linux/ubuntu",
					"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
					"key_sha256": "abc123def456",
				},
			},
		},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs == nil {
		t.Fatal("ExtractSystemRequirements returned nil, want non-nil")
	}

	if len(reqs.Repositories) != 1 {
		t.Fatalf("len(repositories) = %d, want 1", len(reqs.Repositories))
	}

	repo := reqs.Repositories[0]
	if repo.Manager != "apt" {
		t.Errorf("repo.Manager = %q, want %q", repo.Manager, "apt")
	}
	if repo.Type != "repo" {
		t.Errorf("repo.Type = %q, want %q", repo.Type, "repo")
	}
	if repo.URL != "https://download.docker.com/linux/ubuntu" {
		t.Errorf("repo.URL = %q, want %q", repo.URL, "https://download.docker.com/linux/ubuntu")
	}
	if repo.KeyURL != "https://download.docker.com/linux/ubuntu/gpg" {
		t.Errorf("repo.KeyURL = %q, want %q", repo.KeyURL, "https://download.docker.com/linux/ubuntu/gpg")
	}
	if repo.KeySHA256 != "abc123def456" {
		t.Errorf("repo.KeySHA256 = %q, want %q", repo.KeySHA256, "abc123def456")
	}
}

func TestExtractSystemRequirements_AptPPA(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_ppa",
				Params: map[string]interface{}{
					"ppa": "deadsnakes/ppa",
				},
			},
		},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs == nil {
		t.Fatal("ExtractSystemRequirements returned nil, want non-nil")
	}

	if len(reqs.Repositories) != 1 {
		t.Fatalf("len(repositories) = %d, want 1", len(reqs.Repositories))
	}

	repo := reqs.Repositories[0]
	if repo.Manager != "apt" {
		t.Errorf("repo.Manager = %q, want %q", repo.Manager, "apt")
	}
	if repo.Type != "ppa" {
		t.Errorf("repo.Type = %q, want %q", repo.Type, "ppa")
	}
	if repo.PPA != "deadsnakes/ppa" {
		t.Errorf("repo.PPA = %q, want %q", repo.PPA, "deadsnakes/ppa")
	}
}

func TestExtractSystemRequirements_BrewTap(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "brew_tap",
				Params: map[string]interface{}{
					"tap": "homebrew/cask-versions",
				},
			},
		},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs == nil {
		t.Fatal("ExtractSystemRequirements returned nil, want non-nil")
	}

	if len(reqs.Repositories) != 1 {
		t.Fatalf("len(repositories) = %d, want 1", len(reqs.Repositories))
	}

	repo := reqs.Repositories[0]
	if repo.Manager != "brew" {
		t.Errorf("repo.Manager = %q, want %q", repo.Manager, "brew")
	}
	if repo.Type != "tap" {
		t.Errorf("repo.Type = %q, want %q", repo.Type, "tap")
	}
	if repo.Tap != "homebrew/cask-versions" {
		t.Errorf("repo.Tap = %q, want %q", repo.Tap, "homebrew/cask-versions")
	}
}

func TestExtractSystemRequirements_DnfRepo(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "dnf_repo",
				Params: map[string]interface{}{
					"url":    "https://download.docker.com/linux/fedora/docker-ce.repo",
					"gpgkey": "https://download.docker.com/linux/fedora/gpg",
				},
			},
		},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs == nil {
		t.Fatal("ExtractSystemRequirements returned nil, want non-nil")
	}

	if len(reqs.Repositories) != 1 {
		t.Fatalf("len(repositories) = %d, want 1", len(reqs.Repositories))
	}

	repo := reqs.Repositories[0]
	if repo.Manager != "dnf" {
		t.Errorf("repo.Manager = %q, want %q", repo.Manager, "dnf")
	}
	if repo.Type != "repo" {
		t.Errorf("repo.Type = %q, want %q", repo.Type, "repo")
	}
	if repo.URL != "https://download.docker.com/linux/fedora/docker-ce.repo" {
		t.Errorf("repo.URL = %q, want %q", repo.URL, "https://download.docker.com/linux/fedora/docker-ce.repo")
	}
	if repo.KeyURL != "https://download.docker.com/linux/fedora/gpg" {
		t.Errorf("repo.KeyURL = %q, want %q", repo.KeyURL, "https://download.docker.com/linux/fedora/gpg")
	}
}

func TestExtractSystemRequirements_MixedPackagesAndRepos(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_repo",
				Params: map[string]interface{}{
					"url":        "https://download.docker.com/linux/ubuntu",
					"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
					"key_sha256": "abc123",
				},
			},
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"docker-ce", "containerd.io"},
				},
			},
		},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs == nil {
		t.Fatal("ExtractSystemRequirements returned nil, want non-nil")
	}

	// Check packages
	expectedPackages := []string{"docker-ce", "containerd.io"}
	if !reflect.DeepEqual(reqs.Packages["apt"], expectedPackages) {
		t.Errorf("Packages[apt] = %v, want %v", reqs.Packages["apt"], expectedPackages)
	}

	// Check repositories
	if len(reqs.Repositories) != 1 {
		t.Fatalf("len(repositories) = %d, want 1", len(reqs.Repositories))
	}

	repo := reqs.Repositories[0]
	if repo.URL != "https://download.docker.com/linux/ubuntu" {
		t.Errorf("repo.URL = %q, want %q", repo.URL, "https://download.docker.com/linux/ubuntu")
	}
}

func TestExtractSystemRequirements_MultipleRepos(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_repo",
				Params: map[string]interface{}{
					"url":        "https://repo1.example.com",
					"key_url":    "https://repo1.example.com/key.gpg",
					"key_sha256": "hash1",
				},
			},
			{
				Action: "apt_ppa",
				Params: map[string]interface{}{
					"ppa": "user/repo",
				},
			},
			{
				Action: "apt_repo",
				Params: map[string]interface{}{
					"url":        "https://repo2.example.com",
					"key_url":    "https://repo2.example.com/key.gpg",
					"key_sha256": "hash2",
				},
			},
		},
	}

	reqs := ExtractSystemRequirements(plan)

	if reqs == nil {
		t.Fatal("ExtractSystemRequirements returned nil, want non-nil")
	}

	if len(reqs.Repositories) != 3 {
		t.Fatalf("len(repositories) = %d, want 3", len(reqs.Repositories))
	}

	// Verify all repos extracted in order
	if reqs.Repositories[0].URL != "https://repo1.example.com" {
		t.Errorf("repositories[0].URL = %q, want %q", reqs.Repositories[0].URL, "https://repo1.example.com")
	}
	if reqs.Repositories[1].PPA != "user/repo" {
		t.Errorf("repositories[1].PPA = %q, want %q", reqs.Repositories[1].PPA, "user/repo")
	}
	if reqs.Repositories[2].URL != "https://repo2.example.com" {
		t.Errorf("repositories[2].URL = %q, want %q", reqs.Repositories[2].URL, "https://repo2.example.com")
	}
}

func TestExtractSystemRequirements_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Verify ExtractPackages wrapper works correctly
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{
				Action: "apt_install",
				Params: map[string]interface{}{
					"packages": []interface{}{"curl", "wget"},
				},
			},
		},
	}

	// Old function should still work
	packages := ExtractPackages(plan)
	if packages == nil {
		t.Fatal("ExtractPackages returned nil, want map")
	}

	expected := []string{"curl", "wget"}
	if !reflect.DeepEqual(packages["apt"], expected) {
		t.Errorf("packages[apt] = %v, want %v", packages["apt"], expected)
	}
}
