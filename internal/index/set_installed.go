package index

import "context"

// SetInstalled updates the installed flag for all rows belonging to recipe.
// A single UPDATE touches only the rows for the given recipe, avoiding a full
// rebuild. Called by install.Manager after a successful install or remove.
func (idx *sqliteBinaryIndex) SetInstalled(ctx context.Context, recipe string, installed bool) error {
	installedInt := 0
	if installed {
		installedInt = 1
	}
	_, err := idx.db.ExecContext(ctx,
		`UPDATE binaries SET installed = ? WHERE recipe = ?`,
		installedInt, recipe)
	return err
}
