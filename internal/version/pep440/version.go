// Package pep440 evaluates PEP 440 version specifiers against target
// versions. It supports the operator subset that appears in real PyPI
// `requires_python` metadata: >=, <=, >, <, ==, !=, plus ==X.Y.* and
// !=X.Y.* wildcard equality, comma-joined as AND. ~= and === are
// rejected as unsupported.
//
// The evaluator is scoped to PyPI Python-compatibility filtering. It
// is not a complete PEP 440 implementation and intentionally rejects
// inputs outside that scope (local version segments, full prerelease
// comparison semantics, etc.).
package pep440

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Version is a PEP 440 version expressed as integer segments.
// 1- to 4-segment integer versions are accepted; missing trailing
// components compare as 0 (e.g., "3.10" == "3.10.0" == "3.10.0.0").
type Version []int

// Sentinel errors. Wrappers carry the offending input.
var (
	ErrInputTooLong        = errors.New("pep440: input too long")
	ErrNonASCII            = errors.New("pep440: non-ASCII byte in input")
	ErrSegmentTooLarge     = errors.New("pep440: version segment too large")
	ErrUnsupportedOperator = errors.New("pep440: unsupported operator")
	ErrMalformed           = errors.New("pep440: malformed input")
)

const (
	maxSegmentDigits = 6
	maxVersionLen    = 64 // bound on a single version literal
	maxSegments      = 4
)

// ParseVersion parses a 1- to 4-segment integer version string.
// Each segment must be 1-6 ASCII digits and fit in int32.
// Missing trailing components are NOT padded here; comparison handles
// length differences. Returns an empty Version and an error on
// malformed input.
func ParseVersion(s string) (Version, error) {
	if len(s) == 0 {
		return nil, fmt.Errorf("%w: empty version", ErrMalformed)
	}
	if len(s) > maxVersionLen {
		return nil, fmt.Errorf("%w: %d bytes", ErrInputTooLong, len(s))
	}
	if err := requireASCII(s); err != nil {
		return nil, err
	}
	parts := strings.Split(s, ".")
	if len(parts) > maxSegments {
		return nil, fmt.Errorf("%w: %d segments (max %d)", ErrMalformed, len(parts), maxSegments)
	}
	v := make(Version, len(parts))
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("%w: empty segment in %q", ErrMalformed, s)
		}
		if len(p) > maxSegmentDigits {
			return nil, fmt.Errorf("%w: %q (>%d digits)", ErrSegmentTooLarge, p, maxSegmentDigits)
		}
		for j := range len(p) {
			if p[j] < '0' || p[j] > '9' {
				return nil, fmt.Errorf("%w: non-digit in segment %q", ErrMalformed, p)
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
		}
		if n < 0 || int64(n) > math.MaxInt32 {
			return nil, fmt.Errorf("%w: %d out of range", ErrSegmentTooLarge, n)
		}
		v[i] = n
	}
	return v, nil
}

// Compare returns -1 if v < other, 0 if equal, 1 if v > other.
// Missing trailing components in either operand compare as 0 (so
// "3.10" == "3.10.0").
func (v Version) Compare(other Version) int {
	n := max(len(v), len(other))
	for i := range n {
		a, b := 0, 0
		if i < len(v) {
			a = v[i]
		}
		if i < len(other) {
			b = other[i]
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
	}
	return 0
}

// String renders the version as dot-separated integers.
func (v Version) String() string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// hasPrefix reports whether v's leading segments match prefix exactly.
// Used for wildcard equality (==X.Y.*).
func (v Version) hasPrefix(prefix Version) bool {
	if len(prefix) > len(v) {
		return false
	}
	for i, n := range prefix {
		if v[i] != n {
			return false
		}
	}
	return true
}

// requireASCII returns ErrNonASCII if any byte is > 0x7F.
func requireASCII(s string) error {
	for i := range len(s) {
		if s[i] > 0x7F {
			return fmt.Errorf("%w at byte %d", ErrNonASCII, i)
		}
	}
	return nil
}
