Feature: Verify
  Verify that installed tools are working correctly.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: Verify with no arguments
    When I run "tsuku verify"
    Then the exit code is 1

  @critical
  Scenario: Verify a tool that is not installed
    When I run "tsuku verify nonexistent-tool-xyz-12345"
    Then the exit code is 3
    And the error output contains "not found"

  Scenario: Verify an installed tool runs verification command
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku verify actionlint"
    And the output contains "Installation verified"

  Scenario: Verify an installed tool with integrity flag
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku verify actionlint --integrity"
    And the output contains "Installation verified"
