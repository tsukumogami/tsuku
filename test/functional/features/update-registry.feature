Feature: Update Registry
  Refresh cached recipes from the registry.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: Dry run on clean environment shows nothing to refresh
    When I run "tsuku update-registry --dry-run"
    Then the exit code is 0

  Scenario: Update registry on clean environment succeeds
    When I run "tsuku update-registry"
    Then the exit code is 0

  Scenario: Refresh a cached recipe
    When I run "tsuku update-registry --all"
    Then the exit code is 0
    When I run "tsuku update-registry --dry-run"
    Then the exit code is 0
