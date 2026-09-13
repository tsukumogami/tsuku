package project

import "fmt"

// RefusedError reports a .tsuku.toml that discovery found and declined to read.
//
// It is a distinct type from ParseError, not a variant of it, because the two
// say different things and every renderer would otherwise have to tell them
// apart anyway: a parse failure means the file was read and its contents are
// not usable, and a refusal means nothing was read at all. Reporting a refused
// file as unparseable would name a fault in a file the user may not have
// written, and would send `tsuku install` to the exit code it uses for a syntax
// error.
//
// Being a typed error is also what makes the refusal fail closed. Every caller
// already treats a non-nil load error as "there is no usable config here", so
// none of them can act on a refused file, and neither can one added later that
// never learns this type exists.
type RefusedError struct {
	// Dir is the directory containing the file. Activation records it so the
	// next prompt knows this refusal has already been reported.
	Dir string
	// Path is the file itself.
	Path string
	// Reason says what about the file or its surroundings failed the rule.
	Reason string
	// Remedy says what the user can do about it. A refusal that names only a
	// reason leaves someone staring at a repository that stopped working.
	Remedy string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("%q: %s. %s", e.Path, e.Reason, e.Remedy)
}
