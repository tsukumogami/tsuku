package seed

// Source fetches candidate packages from an ecosystem API.
type Source interface {
	Name() string
	Fetch(limit int) ([]Package, error)
}
