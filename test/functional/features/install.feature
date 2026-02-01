Feature: Install
  Install tools and verify they work.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: Install a simple tool
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    And the file "tools/current/actionlint" exists
    And I can run "actionlint -version"

  @critical
  Scenario: Install a tool that does not exist
    When I run "tsuku install nonexistent-tool-xyz-12345"
    Then the exit code is 3

  Scenario: Install with --from generates recipe and installs
    When I run "tsuku install shfmt --from homebrew:shfmt --force --deterministic-only"
    Then the exit code is 0
    And the file "recipes/shfmt.toml" exists
    And I can run "shfmt --version"

  Scenario: Install with --from without tool name shows error
    When I run "tsuku install --from homebrew:jq"
    Then the exit code is 2
    And the error output contains "--from requires exactly one tool name"

  Scenario: List shows installed tool
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku list"
    Then the exit code is 0
    And the output contains "actionlint"
