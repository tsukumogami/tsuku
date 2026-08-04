Feature: Check dependencies
  Check dependencies for a tool recipe.

  Background:
    Given a clean tsuku environment

  Scenario: Check deps for a tool with no dependencies
    When I run "tsuku check-deps go"
    Then the exit code is 0

  Scenario: Check deps for a tool that does not exist
    When I run "tsuku check-deps nonexistent-tool-xyz-12345"
    Then the exit code is 3

  Scenario: Check deps with no arguments
    When I run "tsuku check-deps"
    Then the exit code is 1

  @critical
  Scenario: Check deps JSON output reflects missing dependencies
    # See #2099. ruby's provisionable deps are absent from a clean environment,
    # so all_satisfied has to say so even though the exit code stays 0.
    When I run "tsuku check-deps ruby --json"
    Then the exit code is 0
    And the output contains "\"all_satisfied\": false"
    And the output does not contain "\"all_satisfied\":true"
    And the output does not contain "\"all_satisfied\": true"

  Scenario: Check deps JSON output for tool with no dependencies
    When I run "tsuku check-deps --json go"
    Then the exit code is 0
    And the output contains "all_satisfied"
