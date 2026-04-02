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
