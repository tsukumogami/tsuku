package pep440

import (
	"fmt"
	"strings"
)

// Specifier is a parsed PEP 440 version specifier — one or more
// clauses joined by AND.
type Specifier struct {
	clauses []clause
}

// op identifies a comparison operator.
type op int

const (
	opEQ         op = iota // ==
	opNE                   // !=
	opGE                   // >=
	opGT                   // >
	opLE                   // <=
	opLT                   // <
	opEQWildcard           // ==X.Y.*
	opNEWildcard           // !=X.Y.*
)

func (o op) String() string {
	switch o {
	case opEQ:
		return "=="
	case opNE:
		return "!="
	case opGE:
		return ">="
	case opGT:
		return ">"
	case opLE:
		return "<="
	case opLT:
		return "<"
	case opEQWildcard:
		return "=="
	case opNEWildcard:
		return "!="
	}
	return "?"
}

// clause is one operator + version pair.
type clause struct {
	op  op
	ver Version
}

// String renders a clause in canonical form.
func (c clause) String() string {
	switch c.op {
	case opEQWildcard:
		return c.op.String() + c.ver.String() + ".*"
	case opNEWildcard:
		return c.op.String() + c.ver.String() + ".*"
	default:
		return c.op.String() + c.ver.String()
	}
}

// Input bounds enforced at ParseSpecifier entry.
const (
	maxSpecifierLen = 1024
	maxClauses      = 32
	maxClauseLen    = 256
)

// ParseSpecifier parses a PEP 440 specifier string into a Specifier.
//
// Hardening checks at entry, in order:
//  1. Total length cap of 1024 bytes (ErrInputTooLong).
//  2. ASCII-only (ErrNonASCII).
//  3. Clause count cap of 32 (ErrInputTooLong).
//  4. Per-clause length cap of 256 bytes (ErrInputTooLong).
//  5. Segment-magnitude cap (ErrSegmentTooLarge).
//
// Operators ==, !=, >=, <=, >, < are accepted, plus ==X.Y.* / !=X.Y.*
// wildcards. ~= and === are rejected with ErrUnsupportedOperator.
// Other parse failures return ErrMalformed.
func ParseSpecifier(s string) (Specifier, error) {
	if len(s) > maxSpecifierLen {
		return Specifier{}, fmt.Errorf("%w: %d bytes", ErrInputTooLong, len(s))
	}
	if err := requireASCII(s); err != nil {
		return Specifier{}, err
	}
	if strings.TrimSpace(s) == "" {
		return Specifier{}, fmt.Errorf("%w: empty specifier", ErrMalformed)
	}
	parts := strings.Split(s, ",")
	if len(parts) > maxClauses {
		return Specifier{}, fmt.Errorf("%w: %d clauses", ErrInputTooLong, len(parts))
	}
	clauses := make([]clause, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) > maxClauseLen {
			return Specifier{}, fmt.Errorf("%w: clause longer than %d bytes", ErrInputTooLong, maxClauseLen)
		}
		c, err := parseClause(p)
		if err != nil {
			return Specifier{}, err
		}
		clauses = append(clauses, c)
	}
	return Specifier{clauses: clauses}, nil
}

// parseClause matches the operator (longest-prefix) and parses the
// trailing version. Detects wildcard suffix (.*) for == and !=.
func parseClause(s string) (clause, error) {
	if s == "" {
		return clause{}, fmt.Errorf("%w: empty clause", ErrMalformed)
	}
	// Reject unsupported operators explicitly so the user sees a
	// clear error rather than ErrMalformed.
	if strings.HasPrefix(s, "~=") {
		return clause{}, fmt.Errorf("%w: %q", ErrUnsupportedOperator, "~=")
	}
	if strings.HasPrefix(s, "===") {
		return clause{}, fmt.Errorf("%w: %q", ErrUnsupportedOperator, "===")
	}
	// Longest-prefix match. Two-byte operators first.
	var (
		o    op
		rest string
	)
	switch {
	case strings.HasPrefix(s, ">="):
		o, rest = opGE, s[2:]
	case strings.HasPrefix(s, "<="):
		o, rest = opLE, s[2:]
	case strings.HasPrefix(s, "=="):
		o, rest = opEQ, s[2:]
	case strings.HasPrefix(s, "!="):
		o, rest = opNE, s[2:]
	case strings.HasPrefix(s, ">"):
		o, rest = opGT, s[1:]
	case strings.HasPrefix(s, "<"):
		o, rest = opLT, s[1:]
	default:
		return clause{}, fmt.Errorf("%w: missing operator in %q", ErrMalformed, s)
	}

	rest = strings.TrimSpace(rest)
	wildcard := false
	if strings.HasSuffix(rest, ".*") {
		// Wildcards only valid for == and !=.
		if o != opEQ && o != opNE {
			return clause{}, fmt.Errorf("%w: wildcard %q only valid with == or !=", ErrMalformed, rest)
		}
		rest = rest[:len(rest)-2]
		wildcard = true
	}
	v, err := ParseVersion(rest)
	if err != nil {
		return clause{}, err
	}
	if wildcard {
		switch o {
		case opEQ:
			o = opEQWildcard
		case opNE:
			o = opNEWildcard
		}
	}
	return clause{op: o, ver: v}, nil
}

// Canonical returns a sanitized, ASCII-only canonical form of s
// suitable for inclusion in error messages. If s fails any
// input-hardening check, returns the literal string "<malformed>".
// Never returns raw upstream bytes.
func Canonical(s string) string {
	spec, err := ParseSpecifier(s)
	if err != nil {
		return "<malformed>"
	}
	parts := make([]string, len(spec.clauses))
	for i, c := range spec.clauses {
		parts[i] = c.String()
	}
	return strings.Join(parts, ",")
}
