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
