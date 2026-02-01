Feature: Search
  Search for tools in the registry.

  Background:
    Given a clean tsuku environment

  Scenario: Search with empty string
    When I run "tsuku search ''"
    Then the exit code is 0
    # TODO: should assert output does not suggest "tsuku install" with empty name
    # https://github.com/tsukumogami/tsuku/issues/1293

  Scenario: Search for a known tool
    When I run "tsuku search go"
    Then the exit code is 0
