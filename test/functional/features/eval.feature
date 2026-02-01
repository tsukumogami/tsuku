Feature: Eval
  Evaluate and display a resolved installation plan.

  Background:
    Given a clean tsuku environment

  Scenario: Eval a known recipe
    When I run "tsuku eval go"
    Then the exit code is 0
    And the output contains "go"
