Feature: Developer Environment
  Commands for configuring and diagnosing the developer environment.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: shellenv prints PATH export
    When I run "tsuku shellenv"
    Then the exit code is 0
    And the output contains "export PATH="
    And the output contains ".tsuku-test"

  @critical
  Scenario: doctor reports PATH issues in unconfigured environment
    When I run "tsuku doctor"
    Then the exit code is not 0
    And the output contains "PATH"
