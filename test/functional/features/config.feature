Feature: Config
  Manage tsuku configuration.

  Background:
    Given a clean tsuku environment

  Scenario: List configuration
    When I run "tsuku config"
    Then the exit code is 0

  Scenario: Set an invalid config key
    When I run "tsuku config set nonexistent-key value"
    Then the exit code is 2
    And the error output contains "unknown config key"
