Feature: Create
  Create recipes from package ecosystems.

  Background:
    Given a clean tsuku environment

Scenario: Create recipe to default location
    When I run "tsuku create prettier --from npm --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/prettier.toml" exists

Scenario: Create recipe with --output flag
    When I run "tsuku create prettier --from npm --yes --skip-sandbox --output .tsuku-test/custom/prettier.toml"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "custom/prettier.toml" exists
    And the file "recipes/prettier.toml" does not exist

  Scenario: Create recipe from crates.io
    When I run "tsuku create ripgrep --from crates.io --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ripgrep.toml" exists

  Scenario: Create recipe from rubygems
    When I run "tsuku create jekyll --from rubygems --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/jekyll.toml" exists

  # Auto-install scenarios: only run when toolchain is NOT already present.
  # Excluded from standard CI via ~@requires-no-cargo / ~@requires-no-gem.

  @requires-no-cargo
  Scenario: Create with --yes attempts to auto-install missing toolchain
    When I run "tsuku create ripgrep --from crates.io --yes --skip-sandbox"
    Then the error output contains "requires Cargo"
    And the error output contains "Installing rust"

  @requires-no-gem
  Scenario: Create with --yes attempts to auto-install missing gem toolchain
    When I run "tsuku create jekyll --from rubygems --yes --skip-sandbox"
    Then the error output contains "requires gem"
    And the error output contains "Installing ruby"

  @requires-no-cargo
  Scenario: Create without --yes fails when toolchain missing in non-interactive mode
    When I run "tsuku create ripgrep --from crates.io"
    Then the exit code is 8
    And the error output contains "requires Cargo"

Scenario: Create recipe from pypi
    When I run "tsuku create ruff --from pypi --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ruff.toml" exists

Scenario: Create recipe from go
    When I run "tsuku create github.com/google/uuid --from go --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"

Scenario: Create recipe from cpan
    When I run "tsuku create ack --from cpan --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ack.toml" exists

  Scenario: Create recipe from homebrew
    When I run "tsuku create jq --from homebrew:jq --yes --deterministic-only"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the error output does not contain "was NOT tested in a sandbox"
    And the file "recipes/jq.toml" exists

@macos
  Scenario: Create recipe from cask
    When I run "tsuku create iterm2 --from cask:iterm2 --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/iterm2.toml" exists

  Scenario: Deterministic-only with explicit GitHub builder fails with actionable message
    When I run "tsuku create test-tool --from github:cli/cli --deterministic-only"
    Then the exit code is 9
    And the error output contains "requires LLM for recipe generation"
    And the error output contains "Remove --deterministic-only"

  Scenario: Deterministic-only with discovery GitHub builder fails with actionable message
    When I run "tsuku create fd --deterministic-only"
    Then the exit code is 9
    And the error output contains "requires LLM for recipe generation"
    And the error output contains "Remove --deterministic-only"

  Scenario: Create without --from runs discovery
    When I run "tsuku create nonexistent-tool-xyz"
    Then the exit code is 3
    And the error output contains "could not find"
    And the error output contains "--from"

  Scenario: Create without --from resolves from discovery registry
    When I run "tsuku create jq --deterministic-only --yes"
    Then the exit code is 0
    And the error output contains "Discovered:"
    And the output contains "Recipe created:"
    And the file "recipes/jq.toml" exists
