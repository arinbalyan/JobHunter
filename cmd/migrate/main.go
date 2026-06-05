// Command: migrate
// Standalone database migration tool.
// Applies all pending migrations and exits.
package main

import (
	"fmt"
	"os"

	"github.com/arinbalyan/jobhunter/internal/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: migrate <database_url>")
		fmt.Println("       migrate drop <database_url>  (CAUTION: drops all tables)")
		os.Exit(1)
	}

	args := os.Args[1:]
	url := args[0]

	if len(args) > 1 && args[0] == "drop" {
		url = args[1]
		fmt.Println("⚠️  Dropping all tables...")
		if err := migrations.Drop(url); err != nil {
			fmt.Fprintf(os.Stderr, "Drop failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All tables dropped.")
		return
	}

	fmt.Println("Running migrations...")
	if err := migrations.Run(url); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Migrations complete.")

	// Show current version
	version, dirty, err := migrations.Version(url)
	if err != nil {
		fmt.Printf("Current version: error getting version: %v\n", err)
	} else {
		dirtyStr := ""
		if dirty {
			dirtyStr = " (DIRTY)"
		}
		fmt.Printf("Current schema version: %d%s\n", version, dirtyStr)
	}
}
