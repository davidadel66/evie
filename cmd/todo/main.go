package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type task struct {
	ID          int
	Title       string
	Description string
	CreatedAt   time.Time
	Priority    int
	Status      bool
	DueDate     time.Time
}

type taskList struct {
	Tasks  []task
	NextID int
	Name   string `json:"-"`
	Dir    string `json:"-"`
}

func (list *taskList) Add(t task) {
	t.ID = list.NextID
	t.CreatedAt = time.Now()
	list.Tasks = append(list.Tasks, t)
	list.NextID++
}

func (list *taskList) LoadPath() (string, error) {
	if list.Dir == "" {
		return "", errors.New("directory path is not set")
	}
	if list.Name == "" {
		return "", errors.New("name of task list is not set")
	}

	homePath, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to load user home directory: %w", err)
	}
	return filepath.Join(homePath, list.Dir, list.Name+".json"), nil
}

func (list *taskList) Find(id int) (int, *task) {
	for idx := range list.Tasks {
		if list.Tasks[idx].ID == id {
			return idx, &list.Tasks[idx]
		}
	}
	return -1, nil
}

func (list *taskList) Delete(id int) error {
	i, t := list.Find(id)
	if t == nil {
		return errors.New("task does not exist")
	}
	list.Tasks = append(list.Tasks[:i], list.Tasks[i+1:]...)
	return nil
}

func (list *taskList) Save() error {
	jsonBytes, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return fmt.Errorf("failed to convert tasks into JSON: %w", err)
	}

	localPath, err := list.LoadPath()
	if err != nil {
		return fmt.Errorf("failed to constuct local path: %w", err)
	}

	makeDirError := os.MkdirAll(filepath.Dir(localPath), 0o755)
	if makeDirError != nil {
		return fmt.Errorf("failed to create the directory: %w", makeDirError)
	}

	err = os.WriteFile(localPath, jsonBytes, 0o644)
	if err != nil {
		return fmt.Errorf("failed to save JSON: %w", err)
	}

	return nil
}

func (list *taskList) Load() error {
	localPath, err := list.LoadPath()
	if err != nil {
		return fmt.Errorf("failed to constuct local path: %w", err)
	}
	data, err := os.ReadFile(localPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read tasklist: %w", err)
	}

	return json.Unmarshal(data, list)
}

func (t task) String() string {
	box := "[ ]"
	if t.Status {
		box = "[x]"
	}
	line := fmt.Sprintf("%s #%d %s", box, t.ID, t.Title)

	if t.Priority != 0 {
		line += fmt.Sprintf(" (p%d)", t.Priority)
	}
	if !t.DueDate.IsZero() {
		line += fmt.Sprintf(" due %s", t.DueDate.Format("2006-01-02"))
	}
	return line
}

func usage() {
	fmt.Println(`todo - a simple task manager

  Usage:
    todo list           List all tasks
    todo add <title>    Add a new task
    todo done <id>      Mark a task as done
    todo delete <id>    Delete a task
    todo help           Show this help`)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	list := taskList{
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

		t := task{Title: title, Description: *desc, Priority: *priority}
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
