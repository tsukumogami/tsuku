Feature: Error handling
  Verify error behavior for invalid inputs.

  Background:
    Given a clean tsuku environment

  Scenario: Unknown subcommand
    When I run "tsuku foobar"
    Then the exit code is 1
    And the error output contains "unknown command"

  Scenario: Install with empty tool name
    When I run "tsuku install ''"
    Then the exit code is 3

  Scenario: Install with path traversal in tool name
    When I run "tsuku install ../etc/passwd"
    Then the exit code is 3
    And the error output contains "could not find"

  Scenario: Create with invalid source
    When I run "tsuku create sometool --from invalidsource"
    Then the exit code is 2

  Scenario: Install with no arguments
    # See #2121
    When I run "tsuku install"
    Then the exit code is not 0
    And the error output contains "requires at least 1 arg"

  Scenario: Nonexistent plan file
    When I run "tsuku install --plan /nonexistent/path.json"
    Then the exit code is not 0
