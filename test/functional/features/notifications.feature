Feature: Update notifications
  Notification system with suppression for CI and quiet mode.

  Background:
    Given a clean tsuku environment

  Scenario: quiet flag suppresses update notifications
    When I run "tsuku list --quiet"
    Then the exit code is 0
    And the error output does not contain "update"

  Scenario: notification suppression in CI environment
    When I run "tsuku list"
    Then the exit code is 0
    And the error output does not contain "update"

  Scenario: Out-of-channel notification config set to false
    When I run "tsuku config set updates.notify_out_of_channel false"
    Then the exit code is 0
    When I run "tsuku config get updates.notify_out_of_channel"
    Then the exit code is 0
    And the output contains "false"

  Scenario: Out-of-channel notification config set to true
    When I run "tsuku config set updates.notify_out_of_channel true"
    Then the exit code is 0
    When I run "tsuku config get updates.notify_out_of_channel"
    Then the exit code is 0
    And the output contains "true"
