Feature: Library dependency installation
  Tools with library runtime dependencies should work after installation.

  Background:
    Given a clean tsuku environment

  Scenario: Install tool with library runtime dependency
    # jq depends on oniguruma (a library), which uses the homebrew action.
    # The homebrew action requires patchelf on Linux for RPATH fixing.
    # This verifies that installLibrary resolves action-level dependencies
    # so that patchelf is available during oniguruma installation.
    When I run "tsuku install jq --force"
    Then the exit code is 0
    And I can run "jq --version"
