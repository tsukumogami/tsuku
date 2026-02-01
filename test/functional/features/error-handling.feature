Feature: Error handling
  Verify error behavior for invalid inputs.

  Background:
    Given a clean tsuku environment

  Scenario: Unknown subcommand
    When I run "tsuku foobar"
    Then the exit code is 1
    And the error output contains "unknown command"
