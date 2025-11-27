package bundled

import "embed"

//go:embed recipes/*.toml
var Recipes embed.FS
