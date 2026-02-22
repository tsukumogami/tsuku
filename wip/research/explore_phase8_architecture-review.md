# Architecture Review: DESIGN-sandbox-image-unification

## Sections Reviewed

Solution Architecture (lines 213-273), Implementation Approach (lines 275-310), Consequences (lines 331-356).

---

## 1. Is the architecture clear enough to implement?

**Yes, with two gaps to close during implementation.**

The data flow diagram (lines 219-229) is unambiguous: `container-images.json` at the root feeds three consumers through two mechanisms (go:embed for Go, jq for CI/scripts). The component responsibilities are well-defined. The exported API surface is minimal -- two functions (`FamilyImages()`, `DefaultImage()`).

**Gap A: `SourceBuildSandboxImage` replacement is underspecified.** Line 243 says `requirements.go` replaces both `DefaultSandboxImage` and `SourceBuildSandboxImage` with `containerimages.DefaultImage()`. But these are currently two different images serving two different purposes:

- `DefaultSandboxImage` = `debian:bookworm-slim` (binary installs)
- `SourceBuildSandboxImage` = `ubuntu:22.04` (source builds needing fuller package availability)

The JSON schema maps each family to one image. The design says `DefaultImage()` replaces both, but `ubuntu:22.04` isn't in the JSON at all. The implementation needs to decide:
- Does the debian entry serve double duty (binary + build), dropping the Ubuntu distinction?
- Does the JSON grow a second key (`"debian-build": "ubuntu:22.04"`)?
- Does `SourceBuildSandboxImage` remain a Go constant outside the JSON?

Looking at `requirements.go:82-87`, `ComputeSandboxRequirements` currently uses `DefaultSandboxImage` and `SourceBuildSandboxImage` as separate fallbacks, overriding both when `targetFamily` is set. The simplest correct approach: replace `DefaultSandboxImage` with `containerimages.DefaultImage()` and keep `SourceBuildSandboxImage` as a hardcoded constant (it's a debian-family build variant, not a family-to-image mapping). This should be stated explicitly in the design.

**Gap B: PPA override for Ubuntu.** `container_spec.go:133-135` overrides the base image to `ubuntu:24.04` when PPA repositories are present. This is a conditional override within the debian family, not a separate family entry. The design doesn't address whether this stays as a hardcoded override or moves into the JSON. Since PPA handling is behavior logic (not version configuration), keeping it as code makes sense -- but it should be documented as an intentional exclusion.

**Verdict: Implementable.** An implementer can follow the design. The two gaps are scoping questions, not structural ambiguities. They need one-sentence clarifications, not redesign.

---

## 2. Are there missing components or interfaces?

**No missing components. One interface question.**

The exported API is:

```go
func FamilyImages() map[string]string
func DefaultImage() string
```

`FamilyImages()` returns a `map[string]string` described as "read-only; callers must not modify it." In Go, returning a map provides no read-only enforcement. The current `familyToBaseImage` in `container_spec.go` is a package-level `var`, so callers within the `sandbox` package can already mutate it. Moving it to a function return from another package doesn't change the mutability story -- callers can still write to the returned map.

Options:
1. Return a copy on each call (defensive, small allocation).
2. Document the contract and don't copy (current implicit contract with `familyToBaseImage`).
3. Use an accessor function `ImageForFamily(family string) (string, bool)` instead of exposing the map.

Option 3 is cleaner because it eliminates the mutability question and matches how `container_spec.go:127` actually uses the map (`baseImage, ok := familyToBaseImage[family]`). But this is an advisory concern -- the current pattern is no worse than what exists today.

The `internal/containerimages` package doesn't need any additional interfaces. It's a leaf package with no dependencies beyond the standard library. The dependency direction is correct: `sandbox` imports `containerimages`, not the reverse.

---

## 3. Are the implementation phases correctly sequenced?

**Phase sequencing is correct. One bundling decision is questionable.**

The design says (line 310): "Phases 1 and 2 should ship together in one PR since the Go changes and CI changes are tightly coupled (changing the Go map while leaving CI unchanged would just move the drift location)."

This is wrong in a useful direction. Phases 1 and 2 are **not** tightly coupled from a correctness standpoint. Phase 1 (create `container-images.json`, create `internal/containerimages/`, update sandbox code) can ship alone without creating new drift. The Go code would read from the JSON file and produce the same images CI already hardcodes. The drift between Go source and CI that exists today would be *fixed* (Go would now match CI), even if CI still hardcodes values.

Shipping Phase 1 alone is lower risk: it's a pure Go change testable with `go test ./...`. Phase 2 touches 6+ workflow files and a shell script, each of which needs manual verification in CI. Bundling them means a failing workflow blocks the Go change.

That said, the design's reasoning is pragmatic -- a single PR eliminates the "we shipped Phase 1 and Phase 2 never happened" risk. This is a judgment call, not a structural concern.

Phase 3 (Renovate + drift check) is correctly separated. The drift-check CI job is the structural safety net. Without it, the single-source-of-truth guarantee degrades over time as contributors add new workflows with hardcoded images. The design acknowledges this in the consequences (line 355: "Hardcoded references sneak back in -> CI drift-check job"). It should be treated as a required follow-up, not optional.

---

## 4. Are there simpler alternatives we overlooked?

**One alternative worth noting: update the map in place, skip the package.**

The simplest possible fix for the drift problem today is: update `familyToBaseImage` in `container_spec.go` to match CI, add `// renovate: datasource=docker depName=alpine` comments, and configure Renovate to update the Go file plus workflows via regex. No new package, no JSON file, no build-time copy step.

This was considered and rejected (lines 123-124) because "CI workflows must either read Go source at runtime (brittle parsing) or continue hardcoding their own values." But CI workflows already hardcode values today, and the Renovate regex manager can match patterns in any file type -- including YAML workflow files and Go source. A single Renovate config could update both `familyToBaseImage` entries and workflow `image:` values, achieving the same automated-update goal without the JSON intermediary.

The counter-argument: Renovate becomes a dependency rather than an optimization. If Renovate is down or misconfigured, there's no single file a human can edit. The JSON approach is Renovate-independent. This is a valid tradeoff, and the design made a defensible choice. But the alternative isn't as weak as the rejection makes it sound.

No other overlooked alternatives.

---

## 5. Is the `go generate` / Makefile copy approach sound?

**Sound but fragile. A `go generate` directive is better than a Makefile step.**

The design proposes (line 241): "A `go generate` step copies it into the package directory before compilation. The Makefile already runs `go build`, so adding a copy step is minimal friction."

Three concerns:

**5a. The Makefile doesn't always run.** The Makefile's `build` target runs `go build`. But `go test ./...` (which many developers run directly) does not invoke the Makefile. If someone edits `container-images.json` at the root and runs `go test ./internal/sandbox/...`, the embedded copy is stale. Tests pass against old data. This is acknowledged in the consequences (line 344) but hand-waved with "the Makefile handles this automatically." It doesn't -- only `make build` does.

**5b. `go generate` is more conventional.** Adding a `//go:generate cp ../../container-images.json container-images.json` directive in `internal/containerimages/images.go` means `go generate ./...` syncs the file. This is the standard Go mechanism for this pattern. It's not automatic either (developers must run it), but it's discoverable via `go generate -n ./...` and IDEs flag `go generate` directives.

**5c. CI must copy before build.** The design says CI builds from scratch, so the copy always happens. This is true if CI always invokes `make build`. The current CI in `recipe-validation-core.yml:34` runs `go build` directly, not `make build`. Every CI workflow that builds the binary needs updating to include the copy step, or the Makefile must be used consistently. This is a new CI requirement that the implementation plan doesn't enumerate.

**Better alternative: `go generate` with a CI check.** Add a `go generate` directive, and add a CI step that runs `go generate ./...` followed by `git diff --exit-code` to fail if the embedded copy is out of date. This catches staleness without requiring every build path to include the copy. This pattern is common in Go projects with generated code.

---

## 6. Does `internal/containerimages` add unnecessary indirection?

**No. It's the minimum viable solution given `go:embed` constraints.**

The `go:embed` directive cannot reference files outside the embedding package's directory tree. The JSON file must live at the repo root for CI/script consumers (and for Renovate discoverability). These two constraints force either:

1. A copy mechanism (the file lives at root, gets copied into a package), or
2. A code generation mechanism (the file lives at root, generates Go source in a package).

Either way, a package must exist to own the embedded/generated data. The question is whether that package should be `internal/sandbox/` itself (with the JSON copy placed inside it) or a separate `internal/containerimages/`.

A separate package is better because:

- **`container_spec.go` and `requirements.go` both need the data, but they serve different purposes.** `container_spec.go` maps families to images for building test containers. `requirements.go` selects images based on plan complexity. The image data is shared configuration, not container spec logic or requirements logic.
- **Other consumers may emerge.** If sandbox ever exposes image info through the CLI (`tsuku sandbox images`), or if future CI tooling reads the Go binary's embedded images for verification, a standalone package is the right import point.
- **It follows the existing pattern.** `internal/recipe/embedded.go` owns recipe embedding as a dedicated concern within the recipe package. `internal/builders/llm_integration_test.go` embeds test data local to its package. Each embed is owned by the package that needs it. A `containerimages` package owns container image configuration -- the name maps directly to the concern.

The package is small (one file, two exported functions, ~30 lines of code). The indirection cost is one import statement in `container_spec.go` and `requirements.go`. This is not over-engineering.

---

## Structural Findings

| Finding | Severity | Detail |
|---------|----------|--------|
| `SourceBuildSandboxImage` replacement unspecified | **Blocking** | The design says `DefaultImage()` replaces both constants, but these serve different purposes and map to different images (`debian:bookworm-slim` vs `ubuntu:22.04`). The implementer must guess. Clarify whether `ubuntu:22.04` stays as a constant or enters the JSON. |
| Makefile copy doesn't cover `go test` path | Advisory | Developers running `go test` directly bypass the Makefile. The embedded copy can be stale without detection. A `go generate` directive plus CI staleness check is more reliable. |
| CI build commands need updating | Advisory | `recipe-validation-core.yml:34` runs `go build` directly, not `make build`. Phase 2 needs to ensure all CI build paths include the copy step, or switch to `make build`. |
| Phase 1+2 bundling is optional | Advisory | Phase 1 alone fixes Go-side drift without touching CI. Shipping them separately reduces PR blast radius. |
| PPA override scope unmentioned | Advisory | `ubuntu:24.04` override in `container_spec.go:135` is neither in the JSON nor explicitly excluded from scope. |

---

## Recommendations

1. **Specify what happens to `SourceBuildSandboxImage`.** The simplest correct answer: keep it as a hardcoded constant in `requirements.go`, since it's a debian-family build variant, not a family-to-image mapping. State this explicitly.

2. **Use `go generate` instead of a Makefile copy step.** Add `//go:generate` to `internal/containerimages/` and a CI check that verifies the copy is current (`go generate ./... && git diff --exit-code`). This covers both `make build` and direct `go test` paths.

3. **Consider `ImageForFamily(family string) (string, bool)` instead of `FamilyImages() map[string]string`.** It eliminates the mutability concern and matches the actual usage pattern at `container_spec.go:127`.

4. **State the PPA override is out of scope.** One sentence: "The conditional `ubuntu:24.04` override for PPA repositories remains in `container_spec.go` as behavior logic, not version configuration."
