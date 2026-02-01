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

  Scenario: Discovery fallback finds tool via registry and installs
    When I run "tsuku install shfmt --force --deterministic-only"
    Then the exit code is 0
    And the error output contains "Discovered:"
    And the file "recipes/shfmt.toml" exists
    And I can run "shfmt --version"

  Scenario: Discovery fallback shows actionable error for unknown tool
    When I run "tsuku install nonexistent-discovery-test-xyz"
    Then the exit code is 3
    And the error output contains "could not find"
    And the error output contains "--from"

  Scenario: Install with --from without tool name shows error
    When I run "tsuku install --from homebrew:jq"
    Then the exit code is 2
    And the error output contains "--from requires exactly one tool name"

  # See #1346 - install requires --force for recipes with dynamic checksums
  # When fixed, this should assert exit code 0 and remove the checksum assertion
  Scenario: Install an embedded recipe without force flag requires force
    When I run "tsuku install go"
    Then the exit code is not 0
    And the error output contains "checksum"

  # See #1347 - invalid version gives checksum error instead of version-not-found
  # When fixed, error output should contain "not found" and not "checksum"
  Scenario: Install with invalid version shows checksum error
    When I run "tsuku install go@99.99.99"
    Then the exit code is 6
    And the error output contains "checksum"

  Scenario: List shows installed tool
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku list"
    Then the exit code is 0
    And the output contains "actionlint"
