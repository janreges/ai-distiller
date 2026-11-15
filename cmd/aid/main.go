package main

import (
	"os"

	"github.com/janreges/ai-distiller/internal/cli"
	"github.com/janreges/ai-distiller/internal/version"
)

func main() {
	// Set version for CLI from version package
	cli.Version = version.Version

	// Execute the root command
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
