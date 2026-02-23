# Test Plan: embedded-recipe-musl-coverage

Generated from: docs/designs/DESIGN-embedded-recipe-musl-coverage.md
Issues covered: 5
Total scenarios: 14

---

## Scenario 1: rust.toml has glibc, musl, and darwin when clauses
**ID**: scenario-1
**Testable after**: #1912
**Commands**:
- `grep -c 'libc = \["glibc"\]' internal/recipe/recipes/rust.toml`
- `grep -c 'libc = \["musl"\]' internal/recipe/recipes/rust.toml`
- `grep -c 'os = \["darwin"\]' internal/recipe/recipes/rust.toml`
**Expected**: Each grep returns at least 1 match, confirming rust.toml has all three platform paths (glibc-guarded steps, musl apk_install step, darwin-scoped steps)
**Status**: pending

---

## Scenario 2: rust.toml apk_install step declares correct packages
**ID**: scenario-2
**Testable after**: #1912
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/rust.toml','rb')); apk=[s for s in r['steps'] if s['action']=='apk_install']; assert len(apk)==1; assert sorted(apk[0]['packages'])==['cargo','rust'], f'got {apk[0][\"packages\"]}'; print('PASS')"`
**Expected**: Exactly one apk_install step exists with packages ["rust", "cargo"] (order may vary). Script prints "PASS".
**Status**: pending

---

## Scenario 3: rust.toml verify command uses bare executable name
**ID**: scenario-3
**Testable after**: #1912
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/rust.toml','rb')); assert '{install_dir}' not in r['verify']['command'], f'verify still uses install_dir: {r[\"verify\"][\"command\"]}'; assert 'cargo --version' in r['verify']['command']; print('PASS')"`
**Expected**: The verify command is "cargo --version" (no {install_dir}/bin/ prefix). Script prints "PASS".
**Status**: pending

---

## Scenario 4: python-standalone.toml and perl.toml have musl fallback paths
**ID**: scenario-4
**Testable after**: #1912
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/python-standalone.toml','rb')); apk=[s for s in r['steps'] if s['action']=='apk_install']; assert len(apk)==1 and 'python3' in apk[0]['packages'], f'apk_install: {apk}'; glibc=[s for s in r['steps'] if s.get('when',{}).get('libc')==['glibc']]; assert len(glibc)>=1; print('PASS: python-standalone')"`
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/perl.toml','rb')); apk=[s for s in r['steps'] if s['action']=='apk_install']; assert len(apk)==1 and 'perl' in apk[0]['packages'], f'apk_install: {apk}'; assert '{install_dir}' not in r['verify'].get('command','perl -v'), 'verify still uses install_dir'; print('PASS: perl')"`
**Expected**: Both scripts print PASS confirming each recipe has an apk_install step with the correct Alpine packages, glibc when clauses on existing steps, and bare verify commands.
**Status**: pending

---

## Scenario 5: patchelf.toml has three platform paths with no unconditional steps
**ID**: scenario-5
**Testable after**: #1913
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/patchelf.toml','rb')); steps=r['steps']; unconditional=[s for s in steps if 'when' not in s]; assert len(unconditional)==0, f'{len(unconditional)} unconditional steps remain'; apk=[s for s in steps if s['action']=='apk_install']; assert len(apk)==1 and 'patchelf' in apk[0]['packages']; glibc=[s for s in steps if s.get('when',{}).get('libc')==['glibc']]; darwin=[s for s in steps if s.get('when',{}).get('os')==['darwin']]; assert len(glibc)>=1, 'no glibc steps'; assert len(darwin)>=1, 'no darwin steps'; print('PASS')"`
**Expected**: No unconditional steps remain. Recipe has at least one glibc-scoped step, one musl apk_install step, and one darwin-scoped step. Script prints "PASS".
**Status**: pending

---

## Scenario 6: patchelf.toml verify command uses bare executable name
**ID**: scenario-6
**Testable after**: #1913
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/patchelf.toml','rb')); cmd=r['verify']['command']; assert '{install_dir}' not in cmd, f'verify uses install_dir: {cmd}'; assert 'patchelf --version' in cmd; print('PASS')"`
**Expected**: The verify command is "patchelf --version" without the {install_dir}/bin/ prefix. Script prints "PASS".
**Status**: pending

---

## Scenario 7: nodejs.toml link_dependencies and run_command are glibc-guarded
**ID**: scenario-7
**Testable after**: #1914
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/nodejs.toml','rb')); link=[s for s in r['steps'] if s['action']=='link_dependencies']; run=[s for s in r['steps'] if s['action']=='run_command']; assert all('glibc' in s.get('when',{}).get('libc',[]) for s in link), f'link_dependencies not glibc-guarded: {[s.get(\"when\") for s in link]}'; assert all('glibc' in s.get('when',{}).get('libc',[]) for s in run), f'run_command not glibc-guarded: {[s.get(\"when\") for s in run]}'; print('PASS')"`
**Expected**: Both link_dependencies and run_command steps have libc = ["glibc"] in their when clauses, preventing them from running on musl after apk_install. Script prints "PASS".
**Status**: pending

---

## Scenario 8: nodejs.toml and ruby.toml have musl fallback and bare verify commands
**ID**: scenario-8
**Testable after**: #1914
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/nodejs.toml','rb')); apk=[s for s in r['steps'] if s['action']=='apk_install']; assert len(apk)==1; pkgs=sorted(apk[0]['packages']); assert pkgs==['nodejs','npm'], f'packages: {pkgs}'; assert '{install_dir}' not in r['verify']['command']; print('PASS: nodejs')"`
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/ruby.toml','rb')); apk=[s for s in r['steps'] if s['action']=='apk_install']; assert len(apk)==1; assert 'ruby' in apk[0]['packages']; assert '{install_dir}' not in r['verify']['command']; print('PASS: ruby')"`
**Expected**: nodejs.toml has apk_install with ["nodejs", "npm"], ruby.toml has apk_install with ["ruby"]. Neither verify command contains {install_dir}. Both scripts print PASS.
**Status**: pending

---

## Scenario 9: ruby.toml wrapper script and link_dependencies are glibc-guarded
**ID**: scenario-9
**Testable after**: #1914
**Commands**:
- `python3 -c "import tomllib; r=tomllib.load(open('internal/recipe/recipes/ruby.toml','rb')); link=[s for s in r['steps'] if s['action']=='link_dependencies']; run=[s for s in r['steps'] if s['action']=='run_command']; install=[s for s in r['steps'] if s['action']=='install_binaries']; for label, steps in [('link_dependencies', link), ('run_command', run), ('install_binaries', install)]:
    for s in steps:
        w=s.get('when',{})
        libc=w.get('libc',[])
        # Must not be unconditional on musl -- should have glibc guard or darwin-only scope
        assert 'musl' not in libc or 'glibc' in libc, f'{label} runs on musl: {w}'
print('PASS')"`
**Expected**: None of the link_dependencies, run_command, or install_binaries steps in ruby.toml will execute on musl systems. Script prints "PASS".
**Status**: pending

---

## Scenario 10: All six fixed recipes parse as valid TOML and unit tests pass
**ID**: scenario-10
**Testable after**: #1912, #1913, #1914
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && python3 -c "import tomllib; [tomllib.load(open(f'internal/recipe/recipes/{n}.toml','rb')) for n in ['rust','python-standalone','perl','patchelf','nodejs','ruby']]; print('All 6 recipes parse as valid TOML')"`
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test ./internal/recipe/... -count=1 -timeout=120s`
**Expected**: All six recipe files parse without TOML errors. All existing unit tests in internal/recipe/ pass, confirming no regressions in embedded recipe loading, parsing, or existing coverage checks.
**Status**: pending

---

## Scenario 11: AnalyzeRecipeCoverage detects unguarded glibc actions
**ID**: scenario-11
**Testable after**: #1915
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -run 'TestAnalyzeRecipeCoverage' ./internal/recipe/... -v -count=1`
**Expected**: New unit tests pass that verify: (a) a recipe with an unguarded download action and no apk_install produces a warning, (b) a recipe with guarded download + apk_install produces no warning, (c) a recipe with unguarded download but supported_libc including "musl" produces no warning, (d) a recipe with unconditional homebrew and no apk_install produces a warning.
**Status**: pending

---

## Scenario 12: go.toml and zig.toml declare supported_libc metadata
**ID**: scenario-12
**Testable after**: #1915
**Commands**:
- `python3 -c "import tomllib; g=tomllib.load(open('internal/recipe/recipes/go.toml','rb')); assert sorted(g['metadata']['supported_libc'])==['glibc','musl'], f'go.toml supported_libc: {g[\"metadata\"].get(\"supported_libc\")}'; print('PASS: go.toml')"`
- `python3 -c "import tomllib; z=tomllib.load(open('internal/recipe/recipes/zig.toml','rb')); assert sorted(z['metadata']['supported_libc'])==['glibc','musl'], f'zig.toml supported_libc: {z[\"metadata\"].get(\"supported_libc\")}'; print('PASS: zig.toml')"`
**Expected**: Both go.toml and zig.toml have supported_libc = ["glibc", "musl"] in their metadata, suppressing the structural musl coverage warning for their statically-linked downloads. Both scripts print PASS.
**Status**: pending

---

## Scenario 13: TestTransitiveDepsHavePlatformCoverage passes with structural musl check
**ID**: scenario-13
**Testable after**: #1912, #1913, #1914, #1915
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -run TestTransitiveDepsHavePlatformCoverage ./internal/recipe/... -v -count=1`
**Expected**: The existing integration-level test passes, confirming that all embedded recipes (including the six fixed ones and go/zig with metadata) satisfy both the existing library-focused coverage checks and the new structural musl coverage check. Exit code 0.
**Status**: pending

---

## Scenario 14: CI workflow triggers include embedded recipe paths and stale comment is removed
**ID**: scenario-14
**Testable after**: #1916
**Commands**:
- `grep -q 'internal/recipe/recipes' .github/workflows/test-recipe.yml && echo 'PASS: test-recipe.yml has embedded path' || echo 'FAIL: test-recipe.yml missing embedded path'`
- `grep -q 'internal/recipe/recipes' .github/workflows/test-recipe-changes.yml && echo 'PASS: test-recipe-changes.yml has embedded path (no regression)' || echo 'FAIL: test-recipe-changes.yml lost embedded path'`
- `! grep -q 'musl tests disabled' .github/workflows/test.yml && echo 'PASS: stale musl comment removed from test.yml' || echo 'FAIL: stale musl comment still in test.yml'`
- `! grep -q 'rust-test-musl' .github/workflows/test.yml && echo 'PASS: commented-out rust-test-musl job removed' || echo 'FAIL: commented-out rust-test-musl job still in test.yml'`
**Expected**: (1) test-recipe.yml pull_request paths include internal/recipe/recipes. (2) test-recipe-changes.yml still has the path (no regression). (3) test.yml no longer contains the "musl tests disabled" comment or the commented-out rust-test-musl job. All four checks print PASS.
**Status**: pending

---

## Environment Notes

**Scenarios 1-10, 12, 14** are automatable infrastructure scenarios -- they validate structural properties of TOML files and CI workflow YAML through grep, Python TOML parsing, and Go unit tests. They can run in any environment with Go and Python 3.11+ (for tomllib).

**Scenarios 11, 13** are automatable infrastructure scenarios that validate the Go static analysis code works correctly. They require `go test` to pass.

**Use-case validation**: The true end-to-end use-case scenario -- running `tsuku install <tool>` on an Alpine/musl container and verifying the tool works -- requires an Alpine Docker container with apk available. This is what CI's `test-recipe-changes.yml` workflow validates when it runs on Alpine. The scenarios above verify the structural prerequisites (correct TOML, correct when clauses, correct CI triggers) that make that CI-level Alpine test meaningful. If a manual smoke test is desired:

**Environment**: manual
- Pull the Alpine 3.19 Docker image
- Build tsuku inside the container
- Run `tsuku install <tool>` for each of rust, python-standalone, perl, patchelf, nodejs, ruby
- Verify each tool's binary is callable (e.g., `cargo --version`, `python3 --version`, `node --version`)
- Confirm apk_install was used (check for `/usr/bin/<tool>` rather than `$TSUKU_HOME/tools/<tool>/bin/<tool>`)
