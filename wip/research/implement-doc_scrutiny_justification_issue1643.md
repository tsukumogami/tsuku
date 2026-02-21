# Scrutiny Review: Justification Focus -- Issue #1643

**Issue**: feat(llm): implement tsuku llm download command
**Focus**: justification (evaluate deviation explanations)

## Deviation Analysis

There is exactly one deviation in the requirements mapping:

### Deviation: "exit 2 invalid model"

**Claimed reason**: "gRPC API lacks model validation endpoint"

**Assessment**: **Blocking -- insufficient justification that disguises a skipped requirement.**

#### What the AC requires

The test plan (scenario-12) makes the intent clear: "Invalid model names produce exit code 2 with an error listing valid model names." The `--model` flag accepts a string override, and when the user specifies a model name the addon doesn't recognize, the command should report the error and exit with code 2 (matching `ExitUsage` in the existing exit code table, which is the established convention for invalid arguments).

#### What the deviation claims

The reason states "gRPC API lacks model validation endpoint." This implies the existing `GetStatus` RPC and the proto contract don't provide a way to validate model names.

#### Why the justification is weak

1. **The gRPC API already has a `GetStatus` RPC** that returns `model_name` and `ready` fields. The `--model` flag value is set locally in `llm.go:127` as `effectiveModel` but is never actually sent to the addon server. The current implementation reads `status.ModelName` from `GetStatus` and displays it, but there is no mechanism to tell the addon "I want model X -- is that valid?" The deviation is factually correct that there's no dedicated validation endpoint, but this frames the gap as external API limitation rather than something the implementation could address.

2. **Client-side validation is possible.** The model manifest is a known, fixed set of model names. The design doc describes the manifest: "The manifest maps model names to download URLs and checksums." The Go side could embed or fetch the list of valid model names and validate `--model` before contacting the addon. This was not attempted.

3. **The `--model` flag currently does nothing meaningful.** Looking at `llm.go:126-132`, the `effectiveModel` variable is set from the flag, displayed in output, and used in the prompt description -- but it is never passed to the addon server. The addon receives no instruction about which model to download. The flag creates the appearance of model override without actually overriding anything on the server side.

4. **The reason follows an avoidance pattern.** "Lacks X endpoint" deflects to the API surface when the real issue is that the `--model` flag's override mechanism was not wired through. The deviation doesn't explain what was traded away (users can't select models) or why that trade-off is acceptable (no alternative_considered field). It doesn't mention client-side validation as a possible approach.

#### Severity: Blocking

The `--model` flag is a user-facing feature that the AC explicitly tests. The flag exists, is documented in the command help, and accepts input -- but silently ignores the value when it comes to actual model selection. This is worse than not implementing the flag at all, because it creates a false promise. The "exit 2 on invalid model" AC is a safety net for that flag; without validation, users get no feedback when they typo a model name.

The deviation reason attributes the gap to the gRPC API, but the implementation could validate against a known model list client-side, or could extend the gRPC call to pass the desired model. Neither was attempted, and the deviation doesn't explain why these alternatives were not viable.

## Proportionality Check

9 ACs are marked "implemented", 1 is "deviated". The deviation is on a safety/UX feature (error reporting for invalid input) rather than a core capability. This pattern is consistent with "implement the happy path, defer the error path" -- which is reasonable in many cases but warrants flagging here because the `--model` flag was fully implemented on the input side but not wired through on the action side.

## Summary

One deviation, one blocking finding. The deviation's justification attributes the gap to the gRPC API lacking a validation endpoint, but client-side validation against the known model manifest was a viable alternative that wasn't considered. More critically, the `--model` flag accepts a value and displays it but never actually sends it to the addon server, making the override non-functional. The deviation reason explains why a specific approach wasn't taken, not why the requirement itself couldn't be met.
