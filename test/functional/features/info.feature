Feature: Info
  Show detailed information about a tool.

  Background:
    Given a clean tsuku environment

  Scenario: Info for a tool that does not exist
    When I run "tsuku info nonexistent-tool-xyz-12345"
    Then the exit code is 3
    And the output contains "not found"

  Scenario: Info for a known recipe
    When I run "tsuku info go"
    Then the exit code is 0
    And the output contains "go"

  Scenario: Info for a library recipe without verify section
    When I run "tsuku info hdrhistogram_c"
    Then the exit code is 0
    And the output contains "hdrhistogram_c"
    And the output contains "library"

  Scenario: Info with no arguments
    When I run "tsuku info"
    Then the exit code is 2
