Feature: Search
  Search for tools in the registry.

  Background:
    Given a clean tsuku environment

  @proposed
  Scenario: Search with empty string
    When I run "tsuku search ''"
    Then the exit code is 0
    # Bug: suggests "tsuku install" with empty name. See #1293

  @proposed
  Scenario: Search for a known tool
    When I run "tsuku search go"
    Then the exit code is 0
