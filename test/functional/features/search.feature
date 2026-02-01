Feature: Search
  Search for tools in the registry.

  Background:
    Given a clean tsuku environment

  Scenario: Search with no query does not suggest install
    When I run "tsuku search"
    Then the exit code is 0
    And the output does not contain "tsuku install"

  Scenario: Search for a known tool
    When I run "tsuku search go"
    Then the exit code is 0

  # See #1345 - search should find embedded recipes but currently does not
  Scenario: Search finds embedded recipes
    When I run "tsuku search go"
    Then the exit code is 0
    And the output contains "go"
