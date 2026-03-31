Feature: Background update checks
  The update check infrastructure detects when newer tool versions are available.

  Background:
    Given a clean tsuku environment

  Scenario: check-updates command exists and is hidden
    When I run "tsuku check-updates"
    Then the exit code is 0

  Scenario: Config get updates.enabled defaults to true
    When I run "tsuku config get updates.enabled"
    Then the exit code is 0
    And the output contains "true"

  Scenario: Config set and get updates.check_interval
    When I run "tsuku config set updates.check_interval 12h"
    Then the exit code is 0
    And the output contains "updates.check_interval = 12h"
    When I run "tsuku config get updates.check_interval"
    Then the exit code is 0
    And the output contains "12h"

  Scenario: Config set updates.check_interval with invalid value
    When I run "tsuku config set updates.check_interval invalid"
    Then the exit code is 2
    And the error output contains "must be a duration"

  Scenario: Config set updates.check_interval below minimum
    When I run "tsuku config set updates.check_interval 30m"
    Then the exit code is 2
    And the error output contains "must be between"

  Scenario: Config set and get updates.enabled
    When I run "tsuku config set updates.enabled false"
    Then the exit code is 0
    When I run "tsuku config get updates.enabled"
    Then the exit code is 0
    And the output contains "false"

  Scenario: Config set and get updates.auto_apply
    When I run "tsuku config set updates.auto_apply false"
    Then the exit code is 0
    When I run "tsuku config get updates.auto_apply"
    Then the exit code is 0
    And the output contains "false"

  Scenario: Config set and get updates.notify_out_of_channel
    When I run "tsuku config set updates.notify_out_of_channel false"
    Then the exit code is 0
    When I run "tsuku config get updates.notify_out_of_channel"
    Then the exit code is 0
    And the output contains "false"

  Scenario: Config set and get updates.self_update
    When I run "tsuku config set updates.self_update false"
    Then the exit code is 0
    When I run "tsuku config get updates.self_update"
    Then the exit code is 0
    And the output contains "false"

  Scenario: Cache directory created on check-updates
    When I run "tsuku check-updates"
    Then the exit code is 0
    And the file "cache/updates/.last-check" exists
