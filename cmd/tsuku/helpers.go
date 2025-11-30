package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tsuku-dev/tsuku/internal/errmsg"
	"github.com/tsuku-dev/tsuku/internal/recipe"
)

// loader holds the recipe loader (shared across all commands)
var loader *recipe.Loader

// printInfo prints an informational message unless quiet mode is enabled
func printInfo(a ...interface{}) {
	if !quietFlag {
		fmt.Println(a...)
	}
}

// printInfof prints a formatted informational message unless quiet mode is enabled
func printInfof(format string, a ...interface{}) {
	if !quietFlag {
		fmt.Printf(format, a...)
	}
}

// printJSON marshals the given value to JSON and prints it to stdout
func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		exitWithCode(ExitGeneral)
	}
}

// printError prints an error to stderr with suggestions if available.
// This uses the errmsg package to format errors with actionable suggestions.
func printError(err error) {
	errmsg.Fprint(os.Stderr, err)
}
