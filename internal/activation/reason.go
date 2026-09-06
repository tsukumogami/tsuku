package activation

// Reason says why a declaration in .tsuku.toml did not put a directory on PATH.
//
// There is deliberately no String method. A Reason that rendered itself would
// make fmt.Sprintf("%s: %s", tool, reason) the natural call site, and a single
// templated message is what this type exists to avoid: the reasons carry
// different data, not different words. missing-files needs the version to
// reinstall, bad-form and channel need the declared string the developer is
// about to edit, and no-match is about the installed set. The renderer is a
// switch with a whole sentence per arm and no default.
type Reason int

const (
	// ReasonNoMatch means installation state records no version for the tool
	// that satisfies the declaration.
	ReasonNoMatch Reason = iota
	// ReasonBadForm means the key or the version string is malformed, so the
	// declaration was rejected without consulting installation state.
	ReasonBadForm
	// ReasonChannel means the declaration names a channel (@lts), which cannot
	// be resolved by string matching against recorded versions.
	ReasonChannel
	// ReasonMissingFiles means a recorded version satisfies the declaration but
	// its bin directory is gone or unreadable.
	ReasonMissingFiles
)

// Unhonorable is one declaration that could not be honored.
type Unhonorable struct {
	// Tool is the key exactly as .tsuku.toml wrote it, including any org scope,
	// because that is the line the developer will go and edit.
	Tool string
	// Declared is the version string exactly as .tsuku.toml wrote it.
	Declared string
	// Reason says which of the four cases this is.
	Reason Reason
	// Version is set for ReasonMissingFiles only: the version whose files are
	// gone, which is the one to reinstall.
	Version string
}

// StateUnreadable reports that installation state could not be read at all.
//
// It is a separate field on ActivationResult rather than an entry in the
// Unhonorable slice, and that is load-bearing. It is a property of the read
// that would have classified every declaration, not of any one of them; when it
// fires, nothing was classified. In the slice it would mean writing N identical
// entries and de-duplicating them at render time, and a de-duplication step is
// a line someone can delete -- after which the requirement of exactly one
// message goes red for a reason nobody predicted. Here the wrong output is
// unrepresentable.
type StateUnreadable struct {
	// Tools holds only the declarations that would have needed the read, in
	// PATH order. A bad-form or channel declaration is classified without ever
	// consulting state, so naming it in a message about an unreadable state
	// file would be wrong.
	Tools []string
	// Err is for the debug log, never for the message.
	Err error
}
