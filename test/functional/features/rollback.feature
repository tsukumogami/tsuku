Feature: Rollback and notices
  Auto-apply rollback and failure notice infrastructure.

  Background:
    Given a clean tsuku environment

  Scenario: Rollback command exists
    When I run "tsuku rollback nonexistent"
    Then the exit code is not 0
    And the error output contains "not installed"

  Scenario: Notices command with no notices
    When I run "tsuku notices"
    Then the exit code is 0
    And the output contains "No update failure notices"
