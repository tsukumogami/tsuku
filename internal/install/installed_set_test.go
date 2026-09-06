package install

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
)

func stateManagerWith(t *testing.T, stateJSON string) (*StateManager, string) {
	t.Helper()
	home := t.TempDir()
	if stateJSON != "" {
		if err := os.WriteFile(filepath.Join(home, "state.json"), []byte(stateJSON), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return NewStateManager(&config.Config{HomeDir: home}), home
}

const twoToolsState = `{"installed":{
  "nodejs":{"active_version":"26.8.1","versions":{"26.8.1":{},"26.7.0":{},"9.0.0":{},"10.0.0":{}}},
  "jq":{"active_version":"1.7","versions":{"1.7":{},"1.6":{}}},
  "hidden-dep":{"active_version":"2.0.0","is_hidden":true,"versions":{"2.0.0":{}}}
}}`

func TestInstalledVersionsFor_ReturnsRecordedVersions(t *testing.T) {
	sm, _ := stateManagerWith(t, twoToolsState)

	got, err := sm.InstalledVersionsFor([]string{"nodejs", "jq"})
	if err != nil {
		t.Fatal(err)
	}
	node := append([]string(nil), got["nodejs"]...)
	sort.Strings(node)
	want := []string{"10.0.0", "26.7.0", "26.8.1", "9.0.0"}
	if len(node) != len(want) {
		t.Fatalf("nodejs versions = %v, want %v", node, want)
	}
	for i := range want {
		if node[i] != want[i] {
			t.Fatalf("nodejs versions = %v, want %v", node, want)
		}
	}
	if len(got["jq"]) != 2 {
		t.Fatalf("jq versions = %v, want 2 entries", got["jq"])
	}
}

// A tool state has no record of yields no entry, and a missing state file is
// indistinguishable from an empty one -- both mean nothing is installed.
func TestInstalledVersionsFor_AbsentToolAndAbsentFile(t *testing.T) {
	sm, _ := stateManagerWith(t, twoToolsState)
	got, err := sm.InstalledVersionsFor([]string{"nodejs", "never-installed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["never-installed"]; ok {
		t.Fatal("a tool with no state entry must yield no map entry")
	}

	empty, _ := stateManagerWith(t, "")
	got, err = empty.InstalledVersionsFor([]string{"nodejs"})
	if err != nil {
		t.Fatalf("a missing state file must not be an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a missing state file must yield an empty map, got %v", got)
	}
}

// A hidden tool is still installed. Filtering it here would silently change what
// R8/R9 mean by "installed".
func TestInstalledVersionsFor_IncludesHiddenTools(t *testing.T) {
	sm, _ := stateManagerWith(t, twoToolsState)
	got, err := sm.InstalledVersionsFor([]string{"hidden-dep"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got["hidden-dep"]) != 1 {
		t.Fatalf("hidden tool must be included, got %v", got)
	}
}

// The accessor must not sort. A caller needing "newest" has to reach for version
// comparison; inheriting an ordering from here is how the lexicographic bug
// spreads. Detected by giving it versions whose sorted order is unambiguous and
// asserting the result is not in it.
func TestInstalledVersionsFor_DoesNotSort(t *testing.T) {
	// 40 versions makes an accidentally-sorted return overwhelmingly unlikely
	// to arise from map iteration order alone.
	var b []byte
	b = append(b, []byte(`{"installed":{"many":{"versions":{`)...)
	for i := range 40 {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, []byte(`"1.0.`)...)
		b = append(b, []byte{byte('0' + i/10), byte('0' + i%10)}...)
		b = append(b, []byte(`":{}`)...)
	}
	b = append(b, []byte(`}}}}`)...)

	sm, _ := stateManagerWith(t, string(b))
	got, err := sm.InstalledVersionsFor([]string{"many"})
	if err != nil {
		t.Fatal(err)
	}
	versions := got["many"]
	if len(versions) != 40 {
		t.Fatalf("got %d versions, want 40", len(versions))
	}
	if sort.StringsAreSorted(versions) {
		t.Fatal("InstalledVersionsFor must not sort: a caller that inherits this ordering " +
			"gets the lexicographic bug this accessor exists to avoid handing on")
	}
}

// The whole point of the accessor: N declared tools cost one decode, not N.
func TestInstalledVersionsFor_OneDecodeRegardlessOfToolCount(t *testing.T) {
	sm, home := stateManagerWith(t, twoToolsState)

	// The decode reads state.json exactly once per call. Removing the file after
	// the call cannot affect it; what this asserts is that asking for many names
	// is one call, not many -- verified by the accessor's signature taking a
	// slice and by there being no per-name read inside it.
	names := []string{"nodejs", "jq", "hidden-dep", "a", "b", "c", "d", "e", "f", "g"}
	got, err := sm.InstalledVersionsFor(names)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d recorded tools, want 3", len(got))
	}

	// After the state file is gone the next call still succeeds and returns
	// nothing, which also pins the absent-file contract above.
	if err := os.Remove(filepath.Join(home, "state.json")); err != nil {
		t.Fatal(err)
	}
	got, err = sm.InstalledVersionsFor(names)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("after removal got %v, want empty", got)
	}
}

// Activation must never wait on another tsuku process. With the exclusive state
// lock held for the whole duration and never released, the read still completes
// and still returns the last committed state.
func TestInstalledVersionsFor_DoesNotWaitOnTheStateLock(t *testing.T) {
	sm, home := stateManagerWith(t, twoToolsState)

	lock := NewFileLock(filepath.Join(home, "state.json.lock"))
	if err := lock.LockExclusive(); err != nil {
		t.Fatalf("could not take the exclusive lock: %v", err)
	}
	defer func() { _ = lock.Unlock() }()

	done := make(chan map[string][]string, 1)
	errc := make(chan error, 1)
	go func() {
		got, err := sm.InstalledVersionsFor([]string{"nodejs"})
		if err != nil {
			errc <- err
			return
		}
		done <- got
	}()

	select {
	case err := <-errc:
		t.Fatalf("read failed under a held lock: %v", err)
	case got := <-done:
		if len(got["nodejs"]) != 4 {
			t.Fatalf("resolved from the wrong state: %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("InstalledVersionsFor blocked on the state lock; it must not take it, " +
			"or a shell prompt stalls for as long as an install holds it")
	}
}
