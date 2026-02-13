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

  Scenario: Set and get llm.local_enabled
    When I run "tsuku config set llm.local_enabled true"
    Then the exit code is 0
    And the output contains "llm.local_enabled = true"
    When I run "tsuku config get llm.local_enabled"
    Then the exit code is 0
    And the output contains "true"

  Scenario: Set and get llm.idle_timeout
    When I run "tsuku config set llm.idle_timeout 10m"
    Then the exit code is 0
    And the output contains "llm.idle_timeout = 10m"
    When I run "tsuku config get llm.idle_timeout"
    Then the exit code is 0
    And the output contains "10m"

  Scenario: Set llm.idle_timeout with invalid duration
    When I run "tsuku config set llm.idle_timeout invalid"
    Then the exit code is 2
    And the error output contains "must be a duration"

  Scenario: Set llm.local_enabled with invalid value
    When I run "tsuku config set llm.local_enabled notabool"
    Then the exit code is 2
    And the error output contains "must be true or false"
