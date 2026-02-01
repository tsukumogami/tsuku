Feature: Versions
  List available versions for a tool.

  Background:
    Given a clean tsuku environment

  Scenario: List versions for a known tool
    When I run "tsuku versions go"
    Then the exit code is 0

  Scenario: Versions for a tool that does not exist
    When I run "tsuku versions nonexistent-tool-xyz-12345"
    Then the exit code is 3
    And the error output contains "not found"

  Scenario: Versions with no arguments
    When I run "tsuku versions"
    Then the exit code is 1
