Feature: Recipes
  List available recipes from the registry.

  Background:
    Given a clean tsuku environment

  Scenario: List all available recipes
    When I run "tsuku recipes"
    Then the exit code is 0
    And the output contains "go"

  Scenario: List local recipes when none exist
    When I run "tsuku recipes --local"
    Then the exit code is 0
    And the output contains "No local recipes found"
