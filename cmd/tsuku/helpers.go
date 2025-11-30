package main

import (
	"fmt"

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
