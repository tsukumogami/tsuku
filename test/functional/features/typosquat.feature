@typosquat
Feature: Typosquatting detection
  As a user
  I want to be warned when a tool name is similar to a known registry entry
  So that I can avoid typosquatting attacks

  Background:
    Given a clean tsuku environment

  Scenario: Warn when tool name is similar to registry entry
    When I run "tsuku create rgiprep"
    Then the error output contains "Typosquat warning"
    And the error output contains "similar to"
    And the error output contains "ripgrep"

  Scenario: No warning for exact match
    When I run "tsuku create bat"
    Then the error output does not contain "Typosquat warning"

  Scenario: No warning for distant names
    When I run "tsuku create xyz123"
    Then the error output does not contain "Typosquat warning"
