// Command tool drives the fixture. It has a second sentence.
package main

import (
	"fmt"
	"os"

	"example.com/fixture/internal/alpha"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches on the first word after the binary name.
func run(args []string) int {
	if len(args) == 0 {
		fmt.Println(alpha.Name())
		return 0
	}
	if args[0] == "help" {
		return 0
	}
	if !known(args[0]) {
		return 2
	}
	switch args[0] {
	case "run":
		if len(args) > 1 && args[1] == "now" {
			return 0
		}
		for _, arg := range args[1:] {
			if arg == "--fast" || arg == "" {
				return 1
			}
		}
		return 0
	case "jobs":
		var t tool
		return t.jobs(args[1:])
	case "admin":
		return admin(args)
	}
	return 2
}

// known reports whether word is a command.
func known(word string) bool {
	switch word {
	case "run", "admin", "jobs":
		return true
	}
	return false
}

type tool struct{}

// jobs answers "tool jobs start" and "tool jobs stop". Every clause of
// its switch breaks, so the code after the switch still runs.
func (tool) jobs(args []string) int {
	switch len(args) {
	case 0:
		fmt.Println("no job named")
		break
	default:
		break
	}
	if len(args) > 0 && args[0] == "start" {
		return 0
	}
	if len(args) > 0 && args[0] == "stop" {
		return 0
	}
	return 2
}

// admin answers "tool admin users list" and "tool admin reset|purge".
func admin(args []string) int {
	if len(args) < 2 || args[0] != "admin" {
		return 2
	}
	command := args[1]
	args = args[2:]
	switch {
	case command == "users" && len(args) > 0 && args[0] == "list":
		return 0
	case command == "reset" || command == "purge":
		return 0
	}
	return 2
}
