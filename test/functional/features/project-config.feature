Feature: Project config with org-scoped tools
  Org-scoped recipes in .tsuku.toml use TOML quoted keys.

  Background:
    Given a clean tsuku environment

  Scenario: TOML quoted keys with slash parse and display correctly
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      "tsukumogami/koto" = "latest"
      """
    When I run from "myproject" "tsuku install -y"
    # The output should show the tool list with the bare recipe name
    Then the output contains "koto"

  Scenario: strict_registries blocks unregistered org-scoped source
    And I create home file "config.toml" with content:
      """
      strict_registries = true
      """
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      "someorg/somerepo" = "latest"
      """
    When I run from "myproject" "tsuku install -y"
    Then the exit code is not 0
    And the error output contains "strict_registries"

  Scenario: mixed bare and org-scoped tools display correctly
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      actionlint = ""
      "tsukumogami/koto" = "latest"
      """
    # Don't pass -y so it shows the tool list then waits for confirmation.
    # The non-interactive path (no TTY in test) auto-proceeds.
    When I run from "myproject" "tsuku install -y"
    # Both tools should appear in the output
    Then the output contains "actionlint"
    And the output contains "koto"

  Scenario: no tools declared shows empty message
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      """
    When I run from "myproject" "tsuku install"
    Then the exit code is 0
    And the output contains "No tools declared"

  Scenario: org-scoped key with qualified name parses correctly
    And I create home file "myproject/.tsuku.toml" with content:
      """
      [tools]
      "myorg/registry:mytool" = ""
      """
    When I run from "myproject" "tsuku install -y"
    Then the output contains "mytool"
