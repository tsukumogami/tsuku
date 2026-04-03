Feature: Outdated
  Check for outdated installed tools.

  Background:
    Given a clean tsuku environment

  Scenario: No tools installed
    When I run "tsuku outdated"
    Then the exit code is 0
    And the output contains "No tools installed"

  Scenario: JSON output with no tools installed
    When I run "tsuku outdated --json"
    Then the exit code is 0
    And the output contains "updates"

  Scenario: Help text mentions pin-aware display
    When I run "tsuku outdated --help"
    Then the exit code is 0
    And the output contains "Check for"
