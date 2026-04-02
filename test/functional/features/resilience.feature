Feature: Resilience
  Auto-update hardening: failure suppression, version GC, and doctor checks.

  Background:
    Given a clean tsuku environment

  Scenario: Doctor runs in clean environment
    When I run "tsuku doctor"
    Then the output contains "Checking tsuku environment"

  Scenario: Doctor help mentions environment checks
    When I run "tsuku doctor --help"
    Then the exit code is 0
    And the output contains "environment"

  Scenario: Config set version_retention
    When I run "tsuku config set updates.version_retention 168h"
    Then the exit code is 0
    When I run "tsuku config get updates.version_retention"
    Then the exit code is 0
    And the output contains "168h"

  Scenario: Config set version_retention invalid value
    When I run "tsuku config set updates.version_retention invalid"
    Then the exit code is not 0
    And the error output contains "must be a duration"

  Scenario: Config set version_retention custom value
    When I run "tsuku config set updates.version_retention 720h"
    Then the exit code is 0
    When I run "tsuku config get updates.version_retention"
    Then the exit code is 0
    And the output contains "720h"
