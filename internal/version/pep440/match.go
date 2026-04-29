package pep440

// Satisfies reports whether target satisfies all clauses in s
// (AND semantics). An empty Specifier is considered satisfied (no
// constraints). PyPI metadata never produces an empty specifier in
// practice; ParseSpecifier rejects empty inputs before construction.
func (s Specifier) Satisfies(target Version) bool {
	for _, c := range s.clauses {
		if !c.matches(target) {
			return false
		}
	}
	return true
}

// matches evaluates a single clause against target.
func (c clause) matches(target Version) bool {
	switch c.op {
	case opEQ:
		return target.Compare(c.ver) == 0
	case opNE:
		return target.Compare(c.ver) != 0
	case opGE:
		return target.Compare(c.ver) >= 0
	case opGT:
		return target.Compare(c.ver) > 0
	case opLE:
		return target.Compare(c.ver) <= 0
	case opLT:
		return target.Compare(c.ver) < 0
	case opEQWildcard:
		return target.hasPrefix(c.ver)
	case opNEWildcard:
		return !target.hasPrefix(c.ver)
	}
	return false
}
