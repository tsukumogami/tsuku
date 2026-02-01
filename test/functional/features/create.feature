Feature: Create
  Create recipes from package ecosystems.

  Background:
    Given a clean tsuku environment

  # TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
  Scenario: Create recipe to default location
    When I run "tsuku create prettier --from npm --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/prettier.toml" exists

  # TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
  Scenario: Create recipe with --output flag
    When I run "tsuku create prettier --from npm --yes --skip-sandbox --output .tsuku-test/custom/prettier.toml"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "custom/prettier.toml" exists
    And the file "recipes/prettier.toml" does not exist

  # Requires Cargo toolchain. Expects success once #1287 is resolved.
  @requires-no-cargo
  Scenario: Create recipe from crates.io requires toolchain
    When I run "tsuku create ripgrep --from crates.io --yes --skip-sandbox"
    Then the exit code is 8
    And the error output contains "Cargo is required"

  # Requires gem toolchain. Expects success once #1287 is resolved.
  @requires-no-gem
  Scenario: Create recipe from rubygems requires toolchain
    When I run "tsuku create jekyll --from rubygems --yes --skip-sandbox"
    Then the exit code is 8
    And the error output contains "gem is required"

  # TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
  Scenario: Create recipe from pypi
    When I run "tsuku create ruff --from pypi --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ruff.toml" exists

  # TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
  Scenario: Create recipe from go
    When I run "tsuku create github.com/google/uuid --from go --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"

  # TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
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

  # TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
  @macos
  Scenario: Create recipe from cask
    When I run "tsuku create iterm2 --from cask:iterm2 --yes --skip-sandbox"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/iterm2.toml" exists

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
