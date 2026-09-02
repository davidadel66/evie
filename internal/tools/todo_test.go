package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

func TestTodoToolsReturnFreshLegacyDefinitions(t *testing.T) {
	wantNames := []string{"todo_list", "todo_add"}
	wantSchemas := todoSchemasNamed(BuiltinToolset().Schemas(), wantNames)
	first := TodoTools()
	if got := NewToolset(first).Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("Todo schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}

	first[0].Schema.Function.Name = "changed"
	first[1].Schema.Function.Parameters.Required[0] = "changed"
	first[1].Schema.Function.Parameters.Properties["title"] = first[1].Schema.Function.Parameters.Properties["priority"]
	if got := NewToolset(TodoTools()).Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("mutating one Todo definition snapshot changed the next\n got: %#v\nwant: %#v", got, wantSchemas)
	}
}

func todoSchemasNamed(schemas []openrouter.Tool, names []string) []openrouter.Tool {
	selected := make([]openrouter.Tool, 0, len(names))
	for _, name := range names {
		for _, schema := range schemas {
			if schema.Function.Name == name {
				selected = append(selected, schema)
				break
			}
		}
	}
	return selected
}

func TestTodoSubprocessesHonorParentCancellation(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "todo")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf started > \"$TODO_STARTED\"\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "list",
			call: func(ctx context.Context) error {
				_, err := toDoList(ctx, `{}`)
				return err
			},
		},
		{
			name: "add",
			call: func(ctx context.Context) error {
				_, err := toDoAdd(ctx, `{"title":"cancel me"}`)
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			started := filepath.Join(t.TempDir(), "started")
			t.Setenv("TODO_STARTED", started)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() { done <- tc.call(ctx) }()

			deadline := time.Now().Add(time.Second)
			for {
				if _, err := os.Stat(started); err == nil {
					break
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if time.Now().After(deadline) {
					t.Fatal("todo subprocess did not reach the execution barrier")
				}
				time.Sleep(time.Millisecond)
			}

			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("todo error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("todo subprocess did not stop after parent cancellation")
			}
		})
	}
}
