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
