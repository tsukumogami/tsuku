Feature: Update
  Update a tool to the latest version.

  Background:
    Given a clean tsuku environment

  @proposed
  Scenario: Update with no arguments
    When I run "tsuku update"
    Then the exit code is not 0

  @proposed
  Scenario: Update a tool that is not installed
    When I run "tsuku update nonexistent-tool-xyz-12345"
    Then the exit code is 1
    And the error output contains "not installed"
