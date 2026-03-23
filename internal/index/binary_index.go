// Package index provides command-to-recipe reverse lookup via a SQLite-backed
// binary index stored at $TSUKU_HOME/cache/binary-index.db.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tsukumogami/tsuku/internal/install"
	_ "modernc.org/sqlite" // register the SQLite driver
)

// ErrIndexNotBuilt is returned by Lookup when the index exists but has never
// been populated. Callers should run 'tsuku update-registry' to build the index.
var ErrIndexNotBuilt = errors.New("binary index not built; run 'tsuku update-registry'")

// ErrIndexCorrupt is returned when the database is unreadable or structurally
// broken. Callers should rebuild with 'tsuku update-registry --rebuild-index'.
var ErrIndexCorrupt = errors.New("binary index corrupt; rebuild with 'tsuku update-registry --rebuild-index'")

// StaleIndexWarning is returned (wrapped) by Lookup when the registry directory
// has been updated since the index was last built. Results are still returned;
// this warning is advisory only.
type StaleIndexWarning struct {
	BuiltAt    time.Time
	RegistryAt time.Time
}

// Error implements the error interface.
func (s *StaleIndexWarning) Error() string {
	return fmt.Sprintf(
		"binary index may be stale (built %s, registry updated %s); run 'tsuku update-registry'",
		s.BuiltAt.Format(time.RFC3339),
		s.RegistryAt.Format(time.RFC3339),
	)
}

// BinaryMatch is a single result from BinaryIndex.Lookup.
type BinaryMatch struct {
	Recipe     string // Recipe name (e.g., "jq")
	Command    string // Command name as typed (e.g., "jq")
	BinaryPath string // Path within tool dir (e.g., "bin/jq")
	Installed  bool   // True if any version of Recipe is currently installed
	Source     string // "registry" or "installed" (for custom/local recipes)
}

// Registry provides access to cached recipe data during Rebuild.
// Satisfied by *registry.Registry without requiring an import of internal/registry.
type Registry interface {
	ListCached() ([]string, error)
	GetCached(name string) ([]byte, error)
}

// StateReader provides read access to installed tool state during Rebuild.
// Satisfied by *install.StateManager without requiring direct coupling.
type StateReader interface {
	AllTools() (map[string]install.ToolState, error)
}

// BinaryIndex provides command-to-recipe lookup.
type BinaryIndex interface {
	// Lookup returns recipes that provide the given command, ranked by preference
	// (installed recipes first, then lexicographic by recipe name). Returns an
	// empty slice and nil error when the command is not found. Returns
	// ErrIndexNotBuilt when the index has never been populated.
	Lookup(ctx context.Context, command string) ([]BinaryMatch, error)

	// Rebuild regenerates the index from the recipe registry and installed state.
	Rebuild(ctx context.Context, registry Registry, state StateReader) error

	// SetInstalled updates the installed flag for a single recipe without a full
	// rebuild. Called by install.Manager on install and remove.
	SetInstalled(ctx context.Context, recipe string, installed bool) error

	// Close releases the database connection.
	Close() error
}

// sqliteBinaryIndex is the SQLite-backed implementation of BinaryIndex.
type sqliteBinaryIndex struct {
	db *sql.DB
}

// Open opens or creates the binary index database at dbPath.
//
// If the file does not exist it is created empty (the index is not rebuilt).
// The parent directory of dbPath must already exist; Open returns an error
// rather than silently creating missing parent directories.
//
// Open calls initSchema to create tables if they are absent, then enables
// WAL journal mode and sets a busy timeout of 5 seconds.
func Open(dbPath string) (BinaryIndex, error) {
	// Verify the parent directory exists before attempting to open the DB.
	parent := filepath.Dir(dbPath)
	if _, err := os.Stat(parent); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("index: parent directory does not exist: %s", parent)
		}
		return nil, fmt.Errorf("index: cannot access parent directory %s: %w", parent, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("index: open database %s: %w", dbPath, err)
	}

	// Enable WAL mode for concurrent reads during writes.
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: enable WAL mode: %w", err)
	}

	// Set a 5-second busy timeout to handle transient write locks.
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: set busy timeout: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &sqliteBinaryIndex{db: db}, nil
}

// Close closes the underlying database connection.
func (idx *sqliteBinaryIndex) Close() error {
	return idx.db.Close()
}

// Lookup is implemented in Issue 3.
func (idx *sqliteBinaryIndex) Lookup(_ context.Context, _ string) ([]BinaryMatch, error) {
	return nil, errors.New("index: Lookup not yet implemented")
}

// Rebuild is implemented in Issue 2.
func (idx *sqliteBinaryIndex) Rebuild(_ context.Context, _ Registry, _ StateReader) error {
	return errors.New("index: Rebuild not yet implemented")
}

// SetInstalled is implemented in Issue 4.
func (idx *sqliteBinaryIndex) SetInstalled(_ context.Context, _ string, _ bool) error {
	return errors.New("index: SetInstalled not yet implemented")
}
