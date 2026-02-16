Feature: LLM Runtime
  Manage the local LLM inference runtime.

  Background:
    Given a clean tsuku environment

  Scenario: LLM subcommand shows help
    When I run "tsuku llm --help"
    Then the exit code is 0
    And the output contains "local LLM"
    And the output contains "download"

  Scenario: LLM with no subcommand shows help
    When I run "tsuku llm"
    Then the exit code is 0
    And the output contains "local LLM"

  Scenario: LLM download help shows available flags
    When I run "tsuku llm download --help"
    Then the exit code is 0
    And the output contains "--yes"
    And the output contains "--model"
    And the output contains "--force"
    And the output contains "Download the tsuku-llm addon binary"

  Scenario: LLM download rejects positional arguments
    When I run "tsuku llm download some-arg"
    Then the exit code is 1
    And the error output contains "unknown command"

  Scenario: LLM download rejects unknown flags
    When I run "tsuku llm download --invalid-flag"
    Then the exit code is 1
    And the error output contains "unknown flag"

  Scenario: LLM unknown subcommand shows help
    When I run "tsuku llm notreal"
    Then the exit code is 0
    And the output contains "Available Commands"
    And the output contains "download"
