Feature: Project-level auto-update integration
  .tsuku.toml version constraints take precedence over global auto-update policy.

  Background:
    Given a clean tsuku environment
    And I set env "TSUKU_AUTO_UPDATE" to "1"

  Scenario: exact project pin suppresses auto-apply
    # Set up a cached update entry that would normally trigger auto-apply
    Given I create home file "cache/updates/serve.json" with content:
      """
      {"tool":"serve","active_version":"0.6.0","requested":"","latest_within_pin":"0.7.0","latest_overall":"0.7.0","source":"github","checked_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z","error":""}
      """
    And I create home file "state.json" with content:
      """
      {"installed":{"serve":{"active_version":"0.6.0","versions":{"0.6.0":{"requested":""}}}}}
      """
    # Create a project directory with .tsuku.toml that pins exactly
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      serve = "0.6.0"
      """
    # Run tsuku from the project directory -- exact pin should suppress the update
    When I run from "myproject" "tsuku list"
    Then the exit code is 0
    # Cache entry persists because suppressed entries are not consumed
    And the file "cache/updates/serve.json" exists

  Scenario: prefix project pin blocks cross-major update
    # Cache says version 2.0.0 is available, but project pins to major 1
    Given I create home file "cache/updates/serve.json" with content:
      """
      {"tool":"serve","active_version":"1.0.0","requested":"","latest_within_pin":"2.0.0","latest_overall":"2.0.0","source":"github","checked_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z","error":""}
      """
    And I create home file "state.json" with content:
      """
      {"installed":{"serve":{"active_version":"1.0.0","versions":{"1.0.0":{"requested":""}}}}}
      """
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      serve = "1"
      """
    When I run from "myproject" "tsuku list"
    Then the exit code is 0
    # Cache entry persists because the update was blocked by project pin
    And the file "cache/updates/serve.json" exists

  Scenario: no project config allows auto-apply to attempt update
    # Without .tsuku.toml, auto-apply should attempt the update.
    # Use a fake tool name so the install fails fast on recipe lookup
    # — the test verifies cache consumption, not a real install.
    Given I create home file "cache/updates/fake-auto-apply-tool.json" with content:
      """
      {"tool":"fake-auto-apply-tool","active_version":"0.6.0","requested":"","latest_within_pin":"0.7.0","latest_overall":"0.7.0","source":"github","checked_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z","error":""}
      """
    And I create home file "state.json" with content:
      """
      {"installed":{"fake-auto-apply-tool":{"active_version":"0.6.0","versions":{"0.6.0":{"requested":""}}}}}
      """
    When I run "tsuku list"
    Then the exit code is 0
    # Cache entry consumed by the detached `tsuku apply-updates` process
    # auto-apply spawns from `tsuku list`'s PersistentPreRun. The spawn
    # is async (per #2278), so the assertion polls.
    And the file "cache/updates/fake-auto-apply-tool.json" eventually does not exist within 30 seconds

  Scenario: undeclared tool in project config uses global pin
    # Project config declares python but not the cached tool -- the
    # cached tool uses global pin. Fake name fails install fast.
    Given I create home file "cache/updates/fake-auto-apply-tool.json" with content:
      """
      {"tool":"fake-auto-apply-tool","active_version":"0.6.0","requested":"","latest_within_pin":"0.7.0","latest_overall":"0.7.0","source":"github","checked_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z","error":""}
      """
    And I create home file "state.json" with content:
      """
      {"installed":{"fake-auto-apply-tool":{"active_version":"0.6.0","versions":{"0.6.0":{"requested":""}}}}}
      """
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      python = "3.12"
      """
    # The cached tool is not in .tsuku.toml, so auto-apply should attempt
    # it with the global pin.
    When I run from "myproject" "tsuku list"
    Then the exit code is 0
    # Cache entry consumed asynchronously by the detached `tsuku apply-updates`
    # process (per #2278); poll for absence.
    And the file "cache/updates/fake-auto-apply-tool.json" eventually does not exist within 30 seconds

  Scenario: tsuku commands work from project directory
    Given I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      serve = "0.6.0"
      """
    When I run from "myproject" "tsuku list"
    Then the exit code is 0
    When I run from "myproject" "tsuku recipes"
    Then the exit code is 0
