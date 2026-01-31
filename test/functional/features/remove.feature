Feature: Remove
  Remove an installed tool.

  Background:
    Given a clean tsuku environment

  @proposed
  Scenario: Remove with no arguments
    When I run "tsuku remove"
    Then the exit code is not 0

  @proposed
  Scenario: Remove a tool that is not installed
    When I run "tsuku remove nonexistent-tool-xyz-12345"
    Then the exit code is 1
