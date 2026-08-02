package actions

import (
	"context"
	"fmt"
	"strings"
)

// FallbackURLsParam is the recipe- and plan-level parameter name for the
// ordered list of alternate download sources.
//
// The list sits beside "url" rather than widening it: "url" is read as a
// string in the composite actions, the download_file primitive, the plan
// generator, the {version}-placeholder rules in internal/recipe/hardcoded.go,
// and the URL validator in internal/recipe/validator.go. Keeping it a string
// makes fallback purely additive — absent, nothing anywhere behaves
// differently and no generated plan changes by a byte.
const FallbackURLsParam = "fallback_urls"

// DownloadSources returns the ordered list of sources for a download: the
// primary "url" first, then each entry of "fallback_urls" in declaration
// order. Nothing probes, reorders, or remembers which source answered last
// time — the order is exactly what the recipe author wrote.
//
// Returns a single-element slice for the common case of a step with no
// fallback_urls, so callers can walk one code path regardless.
func DownloadSources(params map[string]interface{}) []string {
	primary, _ := GetString(params, "url")
	fallbacks, _ := GetStringSlice(params, FallbackURLsParam)

	sources := make([]string, 0, 1+len(fallbacks))
	if primary != "" {
		sources = append(sources, primary)
	}
	sources = append(sources, fallbacks...)
	return sources
}

// sourceFailure records one source's failure so the aggregate error can name
// every source that was tried rather than only the last one. Without this a
// maintainer reading CI logs cannot tell an outage from a mirror that dropped
// an old release from a typo in the recipe.
type sourceFailure struct {
	URL string
	Err error
}

// allSourcesFailedError reports that every declared source was tried and
// every one failed.
type allSourcesFailedError struct {
	Failures []sourceFailure
}

func (e *allSourcesFailedError) Error() string {
	if len(e.Failures) == 1 {
		return e.Failures[0].Err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "all %d download sources failed:", len(e.Failures))
	for _, f := range e.Failures {
		fmt.Fprintf(&b, "\n  %s: %v", f.URL, f.Err)
	}
	return b.String()
}

// Unwrap returns the first failure so errors.Is and errors.As keep working
// against the primary source's error, which is what callers checked before
// fallback existed.
func (e *allSourcesFailedError) Unwrap() error {
	if len(e.Failures) == 0 {
		return nil
	}
	return e.Failures[0].Err
}

// newAllSourcesFailedError builds the aggregate error. A single-source
// failure produces the underlying error verbatim, so error messages for the
// overwhelming majority of recipes are unchanged.
func newAllSourcesFailedError(failures []sourceFailure) error {
	if len(failures) == 0 {
		return fmt.Errorf("no download sources configured")
	}
	return &allSourcesFailedError{Failures: failures}
}

// DownloadFirstAvailable downloads from the first source that serves, trying
// them in the order given. It is the plan-time half of fallback;
// downloadFileHTTPWithFallback is the install-time half.
//
// Returns the download result and the URL that actually served it. The serving
// URL is used only for the cache write — it is never written into the
// generated plan, because a plan whose contents depended on which host
// answered would stop being reproducible.
//
// Cancellation short-circuits the walk: a canceled context means the caller
// gave up, not that this source failed, so there is no point trying the next.
func DownloadFirstAvailable(
	ctx context.Context, downloader Downloader, sources []string,
) (*DownloadResult, string, error) {
	var failures []sourceFailure
	for _, source := range sources {
		result, err := downloader.Download(ctx, source)
		if err == nil {
			return result, source, nil
		}
		failures = append(failures, sourceFailure{URL: source, Err: err})
		if ctx.Err() != nil {
			break
		}
	}

	return nil, "", newAllSourcesFailedError(failures)
}

// toInterfaceSlice converts a []string to the []interface{} shape that
// params maps carry, so a fallback list round-trips through JSON plan
// serialization the same way a TOML-parsed list does.
func toInterfaceSlice(values []string) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
