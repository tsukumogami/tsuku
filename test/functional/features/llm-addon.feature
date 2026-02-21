Feature: LLM addon
  Validates that tsuku-llm requires GPU acceleration and reports clear errors
  when hardware requirements are not met.

  Background:
    Given a clean tsuku environment

  @fake-llm-binary
  Scenario: GPU requirement error propagates to the user
    When I run "tsuku create test-tool --from github:cli/cli --yes --skip-sandbox"
    Then the exit code is 1
    And the error output contains "requires a GPU with at least 8 GB VRAM"
