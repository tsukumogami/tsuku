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
    # Without .tsuku.toml, auto-apply should attempt the update
    # (the install will fail since serve isn't a real installed binary, but auto-apply will try)
    Given I create home file "cache/updates/serve.json" with content:
      """
      {"tool":"serve","active_version":"0.6.0","requested":"","latest_within_pin":"0.7.0","latest_overall":"0.7.0","source":"github","checked_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z","error":""}
      """
    And I create home file "state.json" with content:
      """
      {"installed":{"serve":{"active_version":"0.6.0","versions":{"0.6.0":{"requested":""}}}}}
      """
    When I run "tsuku list"
    Then the exit code is 0
    # Cache entry consumed because auto-apply attempted the update (even if it failed)
    And the file "cache/updates/serve.json" does not exist

  Scenario: undeclared tool in project config uses global pin
    # Project config declares python but not serve -- serve uses global pin
    Given I create home file "cache/updates/serve.json" with content:
      """
      {"tool":"serve","active_version":"0.6.0","requested":"","latest_within_pin":"0.7.0","latest_overall":"0.7.0","source":"github","checked_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z","error":""}
      """
    And I create home file "state.json" with content:
      """
      {"installed":{"serve":{"active_version":"0.6.0","versions":{"0.6.0":{"requested":""}}}}}
      """
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      python = "3.12"
      """
    # serve is not in .tsuku.toml, so auto-apply should attempt it with global pin
    When I run from "myproject" "tsuku list"
    Then the exit code is 0
    # Cache entry consumed because serve wasn't suppressed by project config
    And the file "cache/updates/serve.json" does not exist

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
