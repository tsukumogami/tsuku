# Auto-mode decisions — DESIGN download-archive-fallback

Decision questions identified in Phase 1, resolved in Phase 2, cross-validated
in Phase 3. `--auto` was in force, so each landed on the recommended option
without an operator prompt.

| ID | Question | Chosen | Runner-up rejected because |
|----|----------|--------|----------------------------|
| D1 | Recipe-facing shape | `fallback_urls = [...]` alongside `url` | Widening `url` to string-or-array changes the type at ~12 read sites that all call `GetString`. |
| D2 | How the shared funnel absorbs the change | Convert `decomposeDownload`'s six positional parameters to a `downloadSpec` struct, add `FallbackURLs` to it | Adding a seventh positional parameter is what makes the next caller fork the funnel instead of extending it. |
| D3 | Where the list lives in the plan | `params["fallback_urls"]` only; no hoisted `ResolvedStep.FallbackURLs` | A second hoisted field duplicates state that can drift from params, and nothing consumes it. `step.URL` keeps naming the primary and is never rewritten. |
| D4 | Where the failover loop lives | `internal/actions/download_file.go`, wrapping the existing retry loop | The retry loop already owns acquisition policy. A loop in the composite actions would need writing four times. |
| D5 | Download-cache reachability across sources | Candidate-key lookup: `Check` walks the ordered URL list and tries each existing key | Re-deriving the key from the checksum invalidates every cache entry on disk and needs a migration, for no behavioural gain. |
| D6 | Plan format version | Stay at 5 | Bumping to 6 rewrites `format_version` in every golden, which is exactly the mass regeneration R13 exists to prevent. The field is additive and old readers degrade to `url`-only, i.e. today's behaviour. |
| D7 | Emission rule for the new plan field | Emit only when the recipe declares alternates | Always emitting (even as `[]`) changes every golden byte. |
