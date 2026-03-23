package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestOpen_CreatesFile verifies that Open creates the SQLite file when it does
// not exist.
func TestOpen_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("pre-condition: DB file should not exist before Open")
	}

	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	defer idx.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("Open() did not create the DB file: %v", err)
	}
}

// TestOpen_Idempotent verifies that calling Open twice on the same path
// succeeds without error.
func TestOpen_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")

	idx1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open() error = %v, want nil", err)
	}
	idx1.Close()

	idx2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open() error = %v, want nil", err)
	}
	idx2.Close()
}

// TestInitSchema_Idempotent verifies that calling initSchema on a database
// that already has the schema produces no errors.
func TestInitSchema_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	// First call.
	if err := initSchema(db); err != nil {
		t.Fatalf("first initSchema() error = %v, want nil", err)
	}

	// Second call on the same DB.
	if err := initSchema(db); err != nil {
		t.Fatalf("second initSchema() error = %v, want nil", err)
	}
}

// TestClose verifies that Close returns nil after a successful Open.
func TestClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")

	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}

	if err := idx.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// TestOpen_MissingParent verifies that Open returns a non-nil error when the
// parent directory does not exist.
func TestOpen_MissingParent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nonexistent-subdir", "binary-index.db")

	idx, err := Open(dbPath)
	if err == nil {
		idx.Close()
		t.Fatal("Open() with missing parent dir: got nil error, want non-nil")
	}
}
