package project

import "fmt"

// ParseError reports a .tsuku.toml that was found but could not be used.
//
// It carries the directory the file was found in, which is the field callers
// actually need: the shell-activation path records that directory so it can
// tell, on the next prompt, that it has already reported this project. Without
// it the caller would have to re-derive the directory by repeating the
// parent-directory walk that produced the error.
//
// Every failure that belongs to the file itself is this error -- unreadable,
// undecodable, or over the MaxTools cap. A caller distinguishing "this
// project's config is broken" from "something else went wrong" gets all three
// with one errors.As, which is what lets it print one line and carry on rather
// than surfacing a command failure on every shell prompt.
type ParseError struct {
	// Dir is the directory containing the file.
	Dir string
	// Path is the file itself.
	Path string
	// Err is what went wrong.
	Err error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }
