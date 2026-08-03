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

  # The middle step is the point: a plain install of an already-installed
  # version reports success and changes nothing, which is why a broken install
  # had no recovery before --reinstall existed.
  @critical
  Scenario: Reinstall repairs a modified install
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I create home file "tools/current/actionlint" with content:
      """
      corrupted-by-the-test
      """
    And I run "tsuku install actionlint --force"
    Then the exit code is 0
    And the file "tools/current/actionlint" contains "corrupted-by-the-test"
    When I run "tsuku install actionlint --force --reinstall"
    Then the exit code is 0
    And the file "tools/current/actionlint" does not contain "corrupted-by-the-test"
    And I can run "actionlint -version"

  Scenario: Install with --from generates recipe and installs
    When I run "tsuku install shfmt --from homebrew:shfmt --force --deterministic-only"
    Then the exit code is 0
    And the file "recipes/shfmt.toml" exists
    And I can run "shfmt --version"

  # Uses @empty-registry to ensure no recipes exist, forcing discovery to run.
  # Discovery still works because the discovery cache is seeded from recipes/discovery/.
  @empty-registry
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

  Scenario: Install an embedded recipe without force flag
    When I run "tsuku install go"
    Then the exit code is 0
    And the error output does not contain "checksum verification required"

  Scenario: Install with invalid version shows clear error
    When I run "tsuku install go@99.99.99"
    Then the exit code is 6
    And the error output contains "version 99.99.99 not found"

  Scenario: List shows installed tool
    When I run "tsuku install actionlint --force"
    Then the exit code is 0
    When I run "tsuku list"
    Then the exit code is 0
    And the output contains "actionlint"
