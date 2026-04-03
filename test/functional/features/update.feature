Feature: Update
  Update a tool to the latest version.

  Background:
    Given a clean tsuku environment

  Scenario: Update with no arguments
    When I run "tsuku update"
    Then the exit code is not 0

  Scenario: Update a tool that is not installed
    When I run "tsuku update nonexistent-tool-xyz-12345"
    Then the exit code is 1
    And the error output contains "not installed"

  Scenario: Update --all with no tools installed
    When I run "tsuku update --all"
    Then the exit code is 0
    And the output contains "No tools installed"

  Scenario: Update --all and tool name are mutually exclusive
    When I run "tsuku update somtool --all"
    Then the exit code is not 0
    And the error output contains "mutually exclusive"

  Scenario: Update --all help text
    When I run "tsuku update --help"
    Then the exit code is 0
    And the output contains "--all"

  Scenario: Update with no arguments shows usage
    When I run "tsuku update"
    Then the exit code is not 0
