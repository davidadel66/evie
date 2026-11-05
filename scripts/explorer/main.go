// Command explorer extracts Evie's facts from the code, and checks the facts
// file, the system map and the changelog against them (system-explorer.spec.md
// and system-page.spec.md).
//
// Run it from the repository root:
//
//	go run ./scripts/explorer extract   # rewrite the facts file
//	go run ./scripts/explorer check     # report every drift problem
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = "usage: go run ./scripts/explorer extract|check"

func main() {
	os.Exit(run(os.Args[1:], ".", evieDatabases, os.Stdout, os.Stderr))
}

// run executes one subcommand against the repository at root, whose
// databases are created from databases, and returns the process exit code:
// 0 on success, 1 when check finds problems, 2 when the command cannot run.
func run(args []string, root string, databases []databaseSource, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "extract":
		if err := writeFacts(root, databases); err != nil {
			fmt.Fprintf(stderr, "explorer extract: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "wrote %s\n", factsPath)
		return 0
	case "check":
		return runCheck(root, databases, stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}
