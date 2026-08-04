package install

import (
	"path/filepath"
	"sort"

	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// BuildManagedBinaries projects state onto the set of binaries tsuku claims a
// PATH entry for: one entry per visible tool, carrying the binary names its
// active version provides. Pure -- it reads state and nothing else.
//
// Hidden tools are left out, and that exclusion is the whole reason this is a
// projection rather than a range over state.Installed. A hidden tool is
// installed with CreateSymlinks false, so it has no entry in tools/current at
// all; its name resolves to a system copy on a perfectly healthy machine.
// Including hidden tools would make doctor warn once per execution dependency
// for everyone.
//
// A tool whose active version records no binaries falls back to the deprecated
// tool-level list, then to the tool's own name. Binary names are basenames: a
// recipe can record "bin/rg", and it is "rg" that goes on PATH.
func BuildManagedBinaries(state *State) []shellenv.ManagedBinaries {
	if state == nil {
		return nil
	}

	var managed []shellenv.ManagedBinaries
	for name, ts := range state.Installed {
		if ts.IsHidden {
			continue
		}
		if binaries := binariesForActiveVersion(name, ts); len(binaries) > 0 {
			managed = append(managed, shellenv.ManagedBinaries{
				Tool:     name,
				Binaries: binaries,
			})
		}
	}

	// state.Installed is a map, so without this the warning order changes
	// between runs of the same unchanged environment.
	sort.Slice(managed, func(i, j int) bool { return managed[i].Tool < managed[j].Tool })

	return managed
}

// binariesForActiveVersion returns the binary basenames the tool's active
// version puts on PATH, deduplicated and ordered.
func binariesForActiveVersion(name string, ts ToolState) []string {
	activeVersion := ts.ActiveVersion
	if activeVersion == "" {
		activeVersion = ts.Version // legacy state files
	}

	raw := ts.Versions[activeVersion].Binaries
	if len(raw) == 0 {
		raw = ts.Binaries // deprecated tool-level list
	}
	if len(raw) == 0 {
		raw = []string{name}
	}

	seen := make(map[string]bool, len(raw))
	var binaries []string
	for _, entry := range raw {
		base := filepath.Base(entry)
		if base == "" || base == "." || base == string(filepath.Separator) || seen[base] {
			continue
		}
		seen[base] = true
		binaries = append(binaries, base)
	}
	sort.Strings(binaries)

	return binaries
}
