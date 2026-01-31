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

  Scenario: Create recipe fails without --from
    When I run "tsuku create prettier"
    Then the exit code is not 0
    And the error output contains "required"
