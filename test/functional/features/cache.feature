Feature: Cache Management
  Manage the recipe cache with info, cleanup, and clear commands.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: Cache info on fresh environment
    When I run "tsuku cache info"
    Then the exit code is 0

  @critical
  Scenario: Cache info with JSON output
    When I run "tsuku cache info --json"
    Then the exit code is 0
    And the output contains "entries"

  @critical
  Scenario: Cache cleanup dry run on empty cache
    When I run "tsuku cache cleanup --dry-run"
    Then the exit code is 0

  @critical
  Scenario: Cache clear on empty cache
    When I run "tsuku cache clear"
    Then the exit code is 0

  Scenario: Cache info shows entries after install
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku cache info"
    Then the exit code is 0
    And the output does not contain "0 entries"

  Scenario: Cache cleanup dry run after install
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku cache cleanup --dry-run"
    Then the exit code is 0

  Scenario: Cache clear removes download cache entries
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku cache clear"
    Then the exit code is 0
    When I run "tsuku cache info --json"
    Then the exit code is 0
    And the output contains "downloads"
