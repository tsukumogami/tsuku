Feature: Self-update mechanism
  Tsuku can update itself via a manual command or background auto-apply.

  Background:
    Given a clean tsuku environment

  Scenario: self-update command exists and shows help
    When I run "tsuku self-update --help"
    Then the exit code is 0
    And the output contains "Downloads and installs the latest tsuku release"

  Scenario: self-update rejects dev builds
    When I run "tsuku self-update"
    Then the exit code is not 0
    And the error output contains "not available for development builds"

  Scenario: Config get updates.self_update defaults to true
    When I run "tsuku config get updates.self_update"
    Then the exit code is 0
    And the output contains "true"

  Scenario: Config set updates.self_update to false
    When I run "tsuku config set updates.self_update false"
    Then the exit code is 0
    When I run "tsuku config get updates.self_update"
    Then the exit code is 0
    And the output contains "false"

  Scenario: outdated command includes self field in JSON output
    When I run "tsuku outdated --json"
    Then the exit code is 0
