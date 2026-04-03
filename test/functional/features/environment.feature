Feature: Developer Environment
  Commands for configuring and diagnosing the developer environment.

  Background:
    Given a clean tsuku environment

  @critical
  Scenario: shellenv prints PATH export
    When I run "tsuku shellenv"
    Then the exit code is 0
    And the output contains "export PATH="
    And the output contains ".tsuku-test"

  @critical
  Scenario: doctor reports PATH issues in unconfigured environment
    When I run "tsuku doctor"
    Then the exit code is 1
    And the output contains "PATH"

  # --- Env file checks ---

  @critical
  Scenario: doctor reports missing env file
    When I run "tsuku doctor"
    Then the output contains "Env file"
    And the output contains "FAIL"
    And the error output contains "--fix"

  @critical
  Scenario: doctor fix creates env file with shell init cache sourcing
    When I run "tsuku doctor --fix"
    Then the file "env" exists
    And the file "env" contains "_tsuku_init_cache"
    And the file "env" contains "init-cache.bash"
    And the file "env" contains "init-cache.zsh"

  @critical
  Scenario: doctor fix env file passes the env file check
    When I run "tsuku doctor --fix"
    Then the file "env" exists
    When I run "tsuku doctor"
    Then the output contains "Env file"
    And the output does not contain "Env file ... FAIL"

  @critical
  Scenario: doctor detects stale env file
    Given I create home file "env" with content:
      """
      # old content — no init cache sourcing
      export PATH="$HOME/.tsuku/bin:$HOME/.tsuku/tools/current:$PATH"
      """
    When I run "tsuku doctor"
    Then the exit code is 1
    And the output contains "Env file"
    And the output contains "FAIL"
    And the error output contains "--fix"

  @critical
  Scenario: doctor fix repairs stale env file
    Given I create home file "env" with content:
      """
      # old content — no init cache sourcing
      export PATH="$HOME/.tsuku/bin:$HOME/.tsuku/tools/current:$PATH"
      """
    When I run "tsuku doctor --fix"
    Then the file "env" contains "_tsuku_init_cache"
    And the file "env" contains "init-cache.bash"

  # --- Migration ---

  Scenario: doctor fix migrates user exports to env.local
    Given I create home file "env" with content:
      """
      export PATH="$HOME/.tsuku/bin:$HOME/.tsuku/tools/current:$PATH"
      export MY_CUSTOM_TOKEN=abc123
      """
    When I run "tsuku doctor --fix"
    Then the file "env.local" exists
    And the file "env.local" contains "MY_CUSTOM_TOKEN"
    And the file "env" contains "_tsuku_init_cache"
    And the file "env" does not contain "MY_CUSTOM_TOKEN"

  Scenario: doctor fix migration is idempotent
    Given I create home file "env" with content:
      """
      export PATH="$HOME/.tsuku/bin:$HOME/.tsuku/tools/current:$PATH"
      export MY_CUSTOM_TOKEN=abc123
      """
    When I run "tsuku doctor --fix"
    And I run "tsuku doctor --fix"
    Then the file "env.local" contains "MY_CUSTOM_TOKEN"
    And the file "env" does not contain "MY_CUSTOM_TOKEN"

  # --- Shell init cache sourcing (true e2e) ---

  Scenario: shell function from tool init script is available after sourcing env
    Given I create home file "share/shell.d/mytool.bash" with content:
      """
      mytool_hello() { echo "mytool shell integration works"; }
      """
    When I run "tsuku doctor --fix"
    Then the file "share/shell.d/.init-cache.bash" exists
    And the file "share/shell.d/.init-cache.bash" contains "mytool_hello"
    And I source home file "env" and can run "mytool_hello"

  Scenario: init cache contains all installed shell scripts
    Given I create home file "share/shell.d/tool-a.bash" with content:
      """
      tool_a_loaded() { echo "tool-a loaded"; }
      """
    And I create home file "share/shell.d/tool-b.bash" with content:
      """
      tool_b_loaded() { echo "tool-b loaded"; }
      """
    When I run "tsuku doctor --fix"
    Then the file "share/shell.d/.init-cache.bash" contains "tool_a_loaded"
    And the file "share/shell.d/.init-cache.bash" contains "tool_b_loaded"
    And I source home file "env" and can run "tool_a_loaded"
    And I source home file "env" and can run "tool_b_loaded"

  Scenario: env.local customizations are available after sourcing env
    Given I create home file "env.local" with content:
      """
      custom_project_fn() { echo "loaded from env.local"; }
      """
    When I run "tsuku doctor --fix"
    Then I source home file "env" and can run "custom_project_fn"
