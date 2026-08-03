package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
)

// TestGoldenPlansCarryTheStorageMarker keeps the golden plans out of the
// stale-plan warning.
//
// validate-golden-execution.yml feeds every one of these files to `tsuku
// install --plan`, which warns when a plan file carries no storage marker. An
// unmarked golden set means a hundred-odd warnings per run about plans the
// project generated itself, which is how a warning stops being read.
//
// The marker is not part of what a golden file pins -- validate-golden.sh
// strips it from both sides before hashing, so a future PlanStorageVersion bump
// costs no regeneration. That is exactly why nothing else would catch its
// disappearance, and why this test exists.
func TestGoldenPlansCarryTheStorageMarker(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "golden", "plans")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("golden plans not present: %v", err)
	}

	var checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("%s: read: %v", path, readErr)
			return nil
		}
		var plan InstallationPlan
		if unmarshalErr := json.Unmarshal(data, &plan); unmarshalErr != nil {
			t.Errorf("%s: parse: %v", path, unmarshalErr)
			return nil
		}
		checked++
		if plan.StorageVersion < install.PlanStorageVersion {
			t.Errorf("%s: storage_version = %d, want at least %d; installing this golden would warn",
				path, plan.StorageVersion, install.PlanStorageVersion)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if checked == 0 {
		t.Fatal("no golden plans found; this test would pass vacuously")
	}
}
