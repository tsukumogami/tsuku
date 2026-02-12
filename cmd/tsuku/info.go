package main

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

var infoCmd = &cobra.Command{
	Use:   "info <tool> | --recipe <path>",
	Short: "Show detailed information about a tool",
	Long: `Show detailed information about a tool, including description, homepage, and installation status.

Use --deps-only to output only dependencies (one per line for shell consumption).
Use --system with --deps-only to extract system package names instead of recipe names.
Use --family with --system to specify the target Linux family.

Examples:
  tsuku info curl                                    # Show all info about curl
  tsuku info --deps-only curl                        # Show recipe dependencies
  tsuku info --deps-only --system --family alpine zlib  # Show Alpine packages
  apk add $(tsuku info --deps-only --system --family alpine zlib)  # Install deps`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		jsonOutput, _ := cmd.Flags().GetBool("json")
		recipePath, _ := cmd.Flags().GetString("recipe")
		metadataOnly, _ := cmd.Flags().GetBool("metadata-only")
		depsOnly, _ := cmd.Flags().GetBool("deps-only")
		system, _ := cmd.Flags().GetBool("system")
		family, _ := cmd.Flags().GetString("family")

		// Validate mutual exclusivity of --deps-only and --metadata-only
		if depsOnly && metadataOnly {
			printError(fmt.Errorf("cannot specify both --deps-only and --metadata-only"))
			exitWithCode(ExitUsage)
		}

		// Validate --system requires --deps-only
		if system && !depsOnly {
			printError(fmt.Errorf("--system requires --deps-only"))
			exitWithCode(ExitUsage)
		}

		// Validate --family requires --system
		if family != "" && !system {
			printError(fmt.Errorf("--family requires --system"))
			exitWithCode(ExitUsage)
		}

		// Validate family value
		validFamilies := []string{"alpine", "debian", "rhel", "arch", "suse"}
		if family != "" {
			valid := false
			for _, f := range validFamilies {
				if f == family {
					valid = true
					break
				}
			}
			if !valid {
				printError(fmt.Errorf("invalid family %q, must be one of: %s", family, strings.Join(validFamilies, ", ")))
				exitWithCode(ExitUsage)
			}
		}

		// Validate arguments: tool name XOR --recipe
		if recipePath != "" && len(args) > 0 {
			printError(fmt.Errorf("cannot specify both --recipe and a tool name"))
			exitWithCode(ExitUsage)
		}
		if recipePath == "" && len(args) == 0 {
			printError(fmt.Errorf("must specify either a tool name or --recipe flag"))
			exitWithCode(ExitUsage)
		}

		// Load recipe from registry or file
		var r *recipe.Recipe
		var toolName string
		var err error

		if recipePath != "" {
			r, err = loadLocalRecipe(recipePath)
			if err != nil {
				printError(fmt.Errorf("failed to load recipe from %s: %w", recipePath, err))
				exitWithCode(ExitGeneral)
			}
			toolName = r.Metadata.Name
		} else {
			toolName = args[0]
			r, err = loader.Get(toolName, recipe.LoaderOptions{})
			if err != nil {
				fmt.Printf("Tool '%s' not found in registry.\n", toolName)
				exitWithCode(ExitRecipeNotFound)
			}
		}

		// Handle --deps-only mode
		if depsOnly {
			runDepsOnly(cmd, r, toolName, jsonOutput, system, family)
			return
		}

		// Check installation status and get dependencies
		var installedVersion, location string
		var installDeps, runtimeDeps []string
		status := "not_installed"

		// Skip installation state and dependency resolution if metadata-only mode
		if !metadataOnly {
			cfg, err := config.DefaultConfig()
			if err == nil {
				mgr := install.New(cfg)
				tools, _ := mgr.List()

				for _, t := range tools {
					if t.Name == toolName {
						status = "installed"
						installedVersion = t.Version
						location = cfg.ToolDir(toolName, t.Version)
						break
					}
				}

				// Get dependencies from state for installed tools
				if status == "installed" {
					stateMgr := install.NewStateManager(cfg)
					toolState, err := stateMgr.GetToolState(toolName)
					if err == nil && toolState != nil {
						installDeps = toolState.InstallDependencies
						runtimeDeps = toolState.RuntimeDependencies
					}
				}
			}

			// For uninstalled tools, resolve dependencies from recipe
			if status == "not_installed" {
				directDeps := actions.ResolveDependencies(r)
				// Resolve transitive dependencies
				resolvedDeps, err := actions.ResolveTransitive(context.Background(), loader, directDeps, toolName, false)
				if err == nil {
					installDeps = sortedKeys(resolvedDeps.InstallTime)
					runtimeDeps = sortedKeys(resolvedDeps.Runtime)
				} else {
					// Fall back to direct deps if transitive resolution fails
					installDeps = sortedKeys(directDeps.InstallTime)
					runtimeDeps = sortedKeys(directDeps.Runtime)
				}
			}
		}

		// JSON output mode
		if jsonOutput {
			type infoOutput struct {
				Name                 string            `json:"name"`
				Description          string            `json:"description"`
				Homepage             string            `json:"homepage,omitempty"`
				VersionFormat        string            `json:"version_format"`
				VersionSource        string            `json:"version_source"`
				SupportedOS          []string          `json:"supported_os,omitempty"`
				SupportedArch        []string          `json:"supported_arch,omitempty"`
				SupportedLibc        []string          `json:"supported_libc,omitempty"`
				UnsupportedPlatforms []string          `json:"unsupported_platforms,omitempty"`
				UnsupportedReason    string            `json:"unsupported_reason,omitempty"`
				SupportedPlatforms   []recipe.Platform `json:"supported_platforms"`
				Tier                 int               `json:"tier"`
				Type                 string            `json:"type"`
				Status               string            `json:"status,omitempty"`
				InstalledVersion     string            `json:"installed_version,omitempty"`
				Location             string            `json:"location,omitempty"`
				VerifyCommand        string            `json:"verify_command,omitempty"`
				InstallDependencies  []string          `json:"install_dependencies,omitempty"`
				RuntimeDependencies  []string          `json:"runtime_dependencies,omitempty"`
			}
			output := infoOutput{
				Name:                 r.Metadata.Name,
				Description:          r.Metadata.Description,
				Homepage:             r.Metadata.Homepage,
				VersionFormat:        r.Metadata.VersionFormat,
				VersionSource:        r.Version.Source,
				SupportedOS:          r.Metadata.SupportedOS,
				SupportedArch:        r.Metadata.SupportedArch,
				SupportedLibc:        r.Metadata.SupportedLibc,
				UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
				UnsupportedReason:    r.Metadata.UnsupportedReason,
				SupportedPlatforms:   recipe.SupportedPlatforms(r),
				Tier:                 r.Metadata.Tier,
				Type:                 r.Metadata.Type,
				Status:               status,
				InstalledVersion:     installedVersion,
				Location:             location,
				VerifyCommand:        r.Verify.Command,
				InstallDependencies:  installDeps,
				RuntimeDependencies:  runtimeDeps,
			}
			printJSON(output)
			return
		}

		fmt.Printf("Name:           %s\n", r.Metadata.Name)
		fmt.Printf("Description:    %s\n", r.Metadata.Description)
		if r.Metadata.Homepage != "" {
			fmt.Printf("Homepage:       %s\n", r.Metadata.Homepage)
		}
		fmt.Printf("Version Format: %s\n", r.Metadata.VersionFormat)
		fmt.Printf("Version Source: %s\n", r.Version.Source)
		if r.Metadata.Tier > 0 {
			fmt.Printf("Tier:           %d\n", r.Metadata.Tier)
		}
		if r.Metadata.Type != "" {
			fmt.Printf("Type:           %s\n", r.Metadata.Type)
		}

		// Show platform constraints if present
		hasConstraints := len(r.Metadata.SupportedOS) > 0 ||
			len(r.Metadata.SupportedArch) > 0 ||
			len(r.Metadata.SupportedLibc) > 0 ||
			len(r.Metadata.UnsupportedPlatforms) > 0
		if hasConstraints {
			fmt.Printf("Platforms:      %s\n", r.FormatPlatformConstraints())
		}

		// Show installation status (skip in metadata-only mode)
		if !metadataOnly {
			if status == "installed" {
				fmt.Printf("Status:         Installed (v%s)\n", installedVersion)
				fmt.Printf("Location:       %s\n", location)
			} else {
				fmt.Printf("Status:         Not installed\n")
			}
		}

		// Show verification method
		if r.Verify.Command != "" {
			fmt.Printf("Verify Command: %s\n", r.Verify.Command)
		}

		// Show dependencies (skip in metadata-only mode)
		if !metadataOnly {
			if len(installDeps) > 0 {
				fmt.Printf("\nInstall Dependencies:\n")
				for _, dep := range installDeps {
					fmt.Printf("  - %s\n", dep)
				}
			}
			if len(runtimeDeps) > 0 {
				fmt.Printf("\nRuntime Dependencies:\n")
				for _, dep := range runtimeDeps {
					fmt.Printf("  - %s\n", dep)
				}
			}
		}
	},
}

// sortedKeys returns the keys of a map[string]string as a sorted slice.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	infoCmd.Flags().Bool("json", false, "Output in JSON format")
	infoCmd.Flags().String("recipe", "", "Path to a local recipe file (for testing)")
	infoCmd.Flags().Bool("metadata-only", false, "Skip dependency resolution for fast static queries")
	infoCmd.Flags().Bool("deps-only", false, "Output only dependencies (one per line)")
	infoCmd.Flags().Bool("system", false, "With --deps-only: extract system packages instead of recipe names")
	infoCmd.Flags().String("family", "", "With --system: target Linux family (alpine, debian, rhel, arch, suse)")
}

// runDepsOnly handles the --deps-only output mode.
// Outputs dependencies as text (one per line) or JSON.
func runDepsOnly(cmd *cobra.Command, r *recipe.Recipe, toolName string, jsonOutput, system bool, family string) {
	ctx := context.Background()

	// Build target from family or use current platform
	target := buildInfoTarget(family)

	if system {
		// Extract system requirements from transitive dependency tree
		reqs := extractSystemRequirementsFromTree(ctx, r, toolName, target)

		if jsonOutput {
			type repoOutput struct {
				Manager   string `json:"manager"`
				Type      string `json:"type"`
				URL       string `json:"url,omitempty"`
				KeyURL    string `json:"key_url,omitempty"`
				KeySHA256 string `json:"key_sha256,omitempty"`
				PPA       string `json:"ppa,omitempty"`
				Tap       string `json:"tap,omitempty"`
			}
			type depsOutput struct {
				Packages     []string     `json:"packages"`
				Repositories []repoOutput `json:"repositories,omitempty"`
				Family       string       `json:"family,omitempty"`
			}
			var repos []repoOutput
			for _, repo := range reqs.Repositories {
				repos = append(repos, repoOutput{
					Manager:   repo.Manager,
					Type:      repo.Type,
					URL:       repo.URL,
					KeyURL:    repo.KeyURL,
					KeySHA256: repo.KeySHA256,
					PPA:       repo.PPA,
					Tap:       repo.Tap,
				})
			}
			output := depsOutput{
				Packages:     reqs.Packages,
				Repositories: repos,
				Family:       family,
			}
			printJSON(output)
		} else {
			// Text output: backward compatible when no repos, headers when repos present
			hasRepos := len(reqs.Repositories) > 0
			if hasRepos && len(reqs.Packages) > 0 {
				// With repositories, use headers to distinguish sections
				fmt.Println("Packages:")
				for _, pkg := range reqs.Packages {
					fmt.Printf("  %s\n", pkg)
				}
				fmt.Println()
				fmt.Println("Repositories:")
				for _, repo := range reqs.Repositories {
					switch repo.Type {
					case "ppa":
						fmt.Printf("  %s ppa: %s\n", repo.Manager, repo.PPA)
					case "tap":
						fmt.Printf("  %s tap: %s\n", repo.Manager, repo.Tap)
					case "repo":
						fmt.Printf("  %s repo: %s\n", repo.Manager, repo.URL)
						if repo.KeyURL != "" {
							if repo.KeySHA256 != "" {
								fmt.Printf("    key: %s (sha256: %s)\n", repo.KeyURL, repo.KeySHA256)
							} else {
								fmt.Printf("    key: %s\n", repo.KeyURL)
							}
						}
					}
				}
			} else if hasRepos {
				// Repositories only (rare case)
				fmt.Println("Repositories:")
				for _, repo := range reqs.Repositories {
					switch repo.Type {
					case "ppa":
						fmt.Printf("  %s ppa: %s\n", repo.Manager, repo.PPA)
					case "tap":
						fmt.Printf("  %s tap: %s\n", repo.Manager, repo.Tap)
					case "repo":
						fmt.Printf("  %s repo: %s\n", repo.Manager, repo.URL)
						if repo.KeyURL != "" {
							if repo.KeySHA256 != "" {
								fmt.Printf("    key: %s (sha256: %s)\n", repo.KeyURL, repo.KeySHA256)
							} else {
								fmt.Printf("    key: %s\n", repo.KeyURL)
							}
						}
					}
				}
			} else {
				// Packages only - backward compatible one-per-line format
				for _, pkg := range reqs.Packages {
					fmt.Println(pkg)
				}
			}
		}
	} else {
		// Extract recipe dependencies (tsuku-managed)
		directDeps := actions.ResolveDependencies(r)
		resolvedDeps, err := actions.ResolveTransitiveForPlatform(ctx, loader, directDeps, toolName, target.OS(), false)
		if err != nil {
			// Fall back to direct deps if transitive resolution fails
			resolvedDeps = directDeps
		}

		// Combine install and runtime deps
		seen := make(map[string]bool)
		var deps []string
		for name := range resolvedDeps.InstallTime {
			if !seen[name] {
				seen[name] = true
				deps = append(deps, name)
			}
		}
		for name := range resolvedDeps.Runtime {
			if !seen[name] {
				seen[name] = true
				deps = append(deps, name)
			}
		}
		sort.Strings(deps)

		if jsonOutput {
			type depsOutput struct {
				Dependencies []string `json:"dependencies"`
			}
			printJSON(depsOutput{Dependencies: deps})
		} else {
			// Text output: one dependency per line
			for _, dep := range deps {
				fmt.Println(dep)
			}
		}
	}
}

// buildInfoTarget creates a platform.Target from the family flag.
// If family is empty, uses the current platform.
func buildInfoTarget(family string) platform.Target {
	os := runtime.GOOS
	arch := runtime.GOARCH
	platformStr := os + "/" + arch

	// If family is specified, assume Linux
	if family != "" {
		os = "linux"
		platformStr = "linux/" + arch
	}

	// Derive libc from family
	libc := ""
	if os == "linux" {
		if family != "" {
			libc = platform.LibcForFamily(family)
		} else {
			libc = platform.DetectLibc()
		}
	}

	return platform.NewTarget(platformStr, family, libc)
}

// systemRequirementsResult holds extracted system requirements with flattened packages.
type systemRequirementsResult struct {
	Packages     []string                    // Deduplicated package names (flat list)
	Repositories []executor.RepositoryConfig // Repository configurations
}

// extractSystemRequirementsFromTree extracts system requirements from the root recipe
// and all its transitive dependencies.
func extractSystemRequirementsFromTree(ctx context.Context, rootRecipe *recipe.Recipe, rootName string, target platform.Target) *systemRequirementsResult {
	seenPkgs := make(map[string]bool)
	var packages []string
	var repositories []executor.RepositoryConfig

	// Helper to merge requirements
	merge := func(reqs *executor.SystemRequirements) {
		if reqs == nil {
			return
		}
		// Flatten packages from all managers
		for _, pkgs := range reqs.Packages {
			for _, pkg := range pkgs {
				if !seenPkgs[pkg] {
					seenPkgs[pkg] = true
					packages = append(packages, pkg)
				}
			}
		}
		// Add repositories (no deduplication needed as they're action-specific)
		repositories = append(repositories, reqs.Repositories...)
	}

	// Get requirements from the root recipe first
	merge(executor.ExtractSystemRequirementsFromRecipe(rootRecipe, target))

	// Resolve transitive dependencies
	directDeps := actions.ResolveDependencies(rootRecipe)
	resolvedDeps, err := actions.ResolveTransitiveForPlatform(ctx, loader, directDeps, rootName, target.OS(), false)
	if err != nil {
		// If resolution fails, just return what we have
		return &systemRequirementsResult{Packages: packages, Repositories: repositories}
	}

	// Extract requirements from each dependency's recipe
	for depName := range resolvedDeps.InstallTime {
		depRecipe, err := loader.Get(depName, recipe.LoaderOptions{})
		if err != nil {
			continue
		}
		merge(executor.ExtractSystemRequirementsFromRecipe(depRecipe, target))
	}

	return &systemRequirementsResult{Packages: packages, Repositories: repositories}
}
