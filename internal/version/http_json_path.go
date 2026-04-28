package version

import (
	"fmt"
	"strconv"
	"strings"
)

// pathStep is one step in a parsed version path.
//
// Either key or index is meaningful, not both — discriminated by isIndex.
type pathStep struct {
	isIndex bool
	key     string // when isIndex == false
	index   int    // when isIndex == true
}

func (s pathStep) String() string {
	if s.isIndex {
		return fmt.Sprintf("[%d]", s.index)
	}
	return s.key
}

// parseVersionPath parses an http_json version_path expression into a list of
// steps. The grammar accepts dotted access and zero-based array indices:
//
//	version
//	current.version
//	releases[0].version
//	[0].version.openjdk_version
//	releases[0].version.openjdk_version
//
// Leading dots between a `]` and the next key are tolerated:
//
//	releases[0].version       (canonical)
//	releases[0]version        (rejected: missing separator)
//	.version                  (rejected: leading dot at start)
//
// The returned slice is non-nil and non-empty for valid input. An empty path
// is rejected, since `http_json` requires the version_path field.
func parseVersionPath(path string) ([]pathStep, error) {
	if path == "" {
		return nil, fmt.Errorf("path is empty")
	}

	// State machine. afterStep == true means we just produced a step
	// (key or index), so the next char must be `.`, `[`, or end. When
	// afterStep == false, we expect to read either a key (any run of
	// non-`.` non-`[` chars) or an index (`[N]`).
	var steps []pathStep
	i := 0
	afterStep := false

	for i < len(path) {
		c := path[i]

		switch {
		case c == '[':
			// `[` is always valid: starts a new index step from either
			// a fresh-start state or right after another step.
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unterminated `[` at position %d", i)
			}
			end += i
			body := path[i+1 : end]
			if body == "" {
				return nil, fmt.Errorf("empty `[]` at position %d", i)
			}
			n, err := strconv.Atoi(body)
			if err != nil {
				return nil, fmt.Errorf("invalid array index %q at position %d: %w", body, i, err)
			}
			if n < 0 {
				return nil, fmt.Errorf("negative array index %d at position %d", n, i)
			}
			steps = append(steps, pathStep{isIndex: true, index: n})
			i = end + 1
			afterStep = true
			continue

		case c == '.':
			if len(steps) == 0 {
				return nil, fmt.Errorf("path cannot start with `.`")
			}
			if !afterStep {
				// `.` only valid as a separator after a step.
				return nil, fmt.Errorf("unexpected `.` at position %d", i)
			}
			i++
			afterStep = false
			continue

		default:
			if afterStep {
				return nil, fmt.Errorf("expected `.` or `[` at position %d, got %q", i, c)
			}
			start := i
			for i < len(path) && path[i] != '.' && path[i] != '[' {
				i++
			}
			key := path[start:i]
			if key == "" {
				return nil, fmt.Errorf("empty key at position %d", start)
			}
			steps = append(steps, pathStep{isIndex: false, key: key})
			afterStep = true
			continue
		}
	}

	// Trailing `.` leaves afterStep == false with at least one step
	// already produced — that's a dangling separator.
	if !afterStep {
		return nil, fmt.Errorf("unexpected `.` at end of path")
	}

	return steps, nil
}

// walkPath descends the steps through a JSON value (map[string]any /
// []any / leaf) and returns the value at the final step. Errors describe
// the failing step in user-facing terms.
func walkPath(root any, steps []pathStep) (any, error) {
	cur := root
	for i, step := range steps {
		switch {
		case step.isIndex:
			arr, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("step %d (%s): expected array, got %s", i+1, step, jsonKind(cur))
			}
			if step.index >= len(arr) {
				return nil, fmt.Errorf("step %d (%s): index out of range (array has %d element(s))", i+1, step, len(arr))
			}
			cur = arr[step.index]

		default:
			obj, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("step %d (%s): expected object, got %s", i+1, step, jsonKind(cur))
			}
			next, found := obj[step.key]
			if !found {
				return nil, fmt.Errorf("step %d (%s): key not found in object", i+1, step)
			}
			cur = next
		}
	}
	return cur, nil
}

// stringifyLeaf coerces a JSON leaf value into a string representation
// suitable for use as a version. Strings are returned as-is; numbers
// (decoded as float64 by encoding/json) are rendered without trailing
// zeros where possible. Booleans, null, objects, and arrays are
// rejected with a clear error.
func stringifyLeaf(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		// JSON numbers decode to float64. Render integers without a
		// decimal point so a manifest field like {"version": 21}
		// becomes "21", not "21.000000".
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case bool:
		return "", fmt.Errorf("expected string or number at leaf, got bool")
	case nil:
		return "", fmt.Errorf("expected string or number at leaf, got null")
	case map[string]any:
		return "", fmt.Errorf("expected string or number at leaf, got object")
	case []any:
		return "", fmt.Errorf("expected string or number at leaf, got array")
	default:
		return "", fmt.Errorf("expected string or number at leaf, got %T", v)
	}
}

// jsonKind names a JSON value's kind in user-facing terms.
func jsonKind(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}
