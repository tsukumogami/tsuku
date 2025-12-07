package actions

// ActionDeps defines what dependencies an action needs.
// InstallTime deps are needed during `tsuku install`.
// Runtime deps are needed when the installed tool runs.
type ActionDeps struct {
	InstallTime []string // Needed during tsuku install
	Runtime     []string // Needed when tool runs
}

// ActionDependencies is the central registry mapping action names to their dependencies.
// This enables the dependency resolver to collect implicit dependencies from recipes.
var ActionDependencies = map[string]ActionDeps{
	// Ecosystem actions: both install-time and runtime
	// These actions install packages that run on an ecosystem runtime.
	"npm_install":  {InstallTime: []string{"nodejs"}, Runtime: []string{"nodejs"}},
	"pipx_install": {InstallTime: []string{"pipx"}, Runtime: []string{"python"}},
	"gem_install":  {InstallTime: []string{"ruby"}, Runtime: []string{"ruby"}},
	"cpan_install": {InstallTime: []string{"perl"}, Runtime: []string{"perl"}},

	// Compiled binary actions: install-time only
	// These actions compile or download standalone binaries that don't need runtime deps.
	"go_install":    {InstallTime: []string{"go"}, Runtime: nil},
	"cargo_install": {InstallTime: []string{"rust"}, Runtime: nil},
	"nix_install":   {InstallTime: []string{"nix-portable"}, Runtime: nil},

	// Download/extract actions: no dependencies
	// These actions work with files directly using Go's standard library.
	"download":         {InstallTime: nil, Runtime: nil},
	"extract":          {InstallTime: nil, Runtime: nil},
	"chmod":            {InstallTime: nil, Runtime: nil},
	"install_binaries":  {InstallTime: nil, Runtime: nil},
	"install_libraries": {InstallTime: nil, Runtime: nil},
	"set_env":          {InstallTime: nil, Runtime: nil},
	"set_rpath":        {InstallTime: nil, Runtime: nil},
	"run_command":      {InstallTime: nil, Runtime: nil},

	// System package manager actions: no dependencies
	// These rely on system package managers being pre-installed.
	"apt_install":  {InstallTime: nil, Runtime: nil},
	"yum_install":  {InstallTime: nil, Runtime: nil},
	"brew_install": {InstallTime: nil, Runtime: nil},

	// Composite actions: no dependencies
	// These combine primitives and inherit their (lack of) dependencies.
	"download_archive":  {InstallTime: nil, Runtime: nil},
	"github_archive":    {InstallTime: nil, Runtime: nil},
	"github_file":       {InstallTime: nil, Runtime: nil},
	"hashicorp_release": {InstallTime: nil, Runtime: nil},
	"homebrew_bottle":   {InstallTime: nil, Runtime: nil},
}

// GetActionDeps returns the dependencies for an action by name.
// Returns an empty ActionDeps if the action is not found.
func GetActionDeps(actionName string) ActionDeps {
	if deps, ok := ActionDependencies[actionName]; ok {
		return deps
	}
	return ActionDeps{}
}
