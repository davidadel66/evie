// Command todo is the CLI frontend over internal/todo: a small task
// manager with stable IDs and JSON persistence in ~/.todo/<name>.json.
// TODO_DIR and TODO_NAME select which list to operate on, so multiple
// named lists coexist with zero config for the common case. This file
// owns every printed line; the engine stays silent.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/davidadel66/moussa/internal/todo"
)

// usage prints the command summary.
func usage() {
	fmt.Println(`todo - a simple task manager

  Usage:
    todo list           List all tasks
    todo add <title>    Add a new task
    todo done <id>      Mark a task as done
    todo delete <id>    Delete a task
    todo help           Show this help`)
}

// envOr reads an environment variable with a default — the repo's
// convention for ambient config, so the common case needs zero setup.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// main loads the list, dispatches on the subcommand, and saves once at
// the end for every state-changing command — list and help return early
// specifically to skip that save.
func main() {
	list := todo.TaskList{
		Dir:  envOr("TODO_DIR", ".todo/"),
		Name: envOr("TODO_NAME", "DayToDay"),
	}

	if err := list.Load(); err != nil {
		fmt.Printf("failed to load tasks: %v\n", err)
		return
	}

	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "list":
		for _, t := range list.Tasks {
			fmt.Println(t.String())
		}
		return

	case "help":
		usage()
		return

	case "add":
		fs := flag.NewFlagSet("add", flag.ExitOnError)
		priority := fs.Int("priority", 0, "priority 1-5 (0 = none)")
		desc := fs.String("desc", "", "longer description")
		due := fs.String("due", "", "due date as YYYY-MM-DD")
		fs.Parse(os.Args[2:])

		if fs.NArg() > 1 {
			fmt.Println(`error: put flags before the title, e.g. todo add --priority 3 "buy milk"`)
			return
		}

		title := fs.Arg(0)
		if title == "" {
			fmt.Println("usage: todo add [--priority N] [--due YYYY-MM-DD] [--desc text] <title>")
			return
		}

		t := todo.Task{Title: title, Description: *desc, Priority: *priority}
		if *due != "" {
			parsed, err := time.Parse("2006-01-02", *due)
			if err != nil {
				fmt.Printf("bad --due %q (want YYYY-MM-DD)\n", *due)
				return
			}
			t.DueDate = parsed
		}
		list.Add(t)
		fmt.Printf("Added: %s\n", title)

	case "done":
		if len(os.Args) < 3 {
			fmt.Println("usage: todo done <id>")
			return
		}

		id, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Printf("invalid id: %q\n", os.Args[2])
			return
		}
		_, t := list.Find(id)
		if t == nil {
			fmt.Println("task does not exist")
			return
		}
		t.Status = true

	case "delete":
		if len(os.Args) < 3 {
			fmt.Println("usage: todo delete <id>")
			return
		}
		id, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Printf("invalid id: %q\n", os.Args[2])
			return
		}
		if err := list.Delete(id); err != nil {
			fmt.Println(err)
			return
		}

	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		usage()
		return
	}

	if err := list.Save(); err != nil {
		fmt.Printf("failed to save: %v\n", err)
	}
}
