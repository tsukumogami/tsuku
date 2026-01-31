Feature: Create
  Create recipes from package ecosystems.

  Background:
    Given a clean tsuku environment

  Scenario: Create recipe to default location
    When I run "tsuku create prettier --from npm --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/prettier.toml" exists

  Scenario: Create recipe with --output flag
    When I run "tsuku create prettier --from npm --yes --output .tsuku-test/custom/prettier.toml"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "custom/prettier.toml" exists
    And the file "recipes/prettier.toml" does not exist

  Scenario: Create recipe from crates.io
    When I run "tsuku create ripgrep --from crates.io --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ripgrep.toml" exists

  Scenario: Create recipe from rubygems
    When I run "tsuku create jekyll --from rubygems --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/jekyll.toml" exists

  Scenario: Create recipe from pypi
    When I run "tsuku create ruff --from pypi --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ruff.toml" exists

  Scenario: Create recipe from go
    When I run "tsuku create github.com/google/uuid --from go --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"

  Scenario: Create recipe from cpan
    When I run "tsuku create ack --from cpan --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/ack.toml" exists

  Scenario: Create recipe from homebrew
    When I run "tsuku create jq --from homebrew:jq --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/jq.toml" exists

  @macos
  Scenario: Create recipe from cask
    When I run "tsuku create iterm2 --from cask:iterm2 --yes"
    Then the exit code is 0
    And the output contains "Recipe created:"
    And the file "recipes/iterm2.toml" exists

  Scenario: Create recipe fails without --from
    When I run "tsuku create prettier"
    Then the exit code is 1
    And the error output contains "required"
