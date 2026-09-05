// memory-stage4-pilot measures disposable infrastructure fixtures. It does not
// select a learned extractor, adjudicate quality, or enable production memory.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type workload struct {
	Mode           string `json:"mode"`
	RetainedEvents int    `json:"retained_events"`
	SourceBytes    int    `json:"source_bytes"`
	GraphClaims    int    `json:"graph_claims"`
	Scopes         int    `json:"scopes"`
	DelayMS        int    `json:"scripted_service_delay_ms"`
	Processes      int    `json:"worker_processes"`
	Turns          int    `json:"foreground_turns"`
	BackfillRoots  int    `json:"backfill_roots"`
}

func (w workload) validate() error {
	if w.Mode != "disabled" && w.Mode != "new" && w.Mode != "history" {
		return errors.New("mode must be disabled, new, or history")
	}
	if w.RetainedEvents < 0 || w.RetainedEvents > 1000000 || w.SourceBytes < 64 || w.SourceBytes > 12000 || w.GraphClaims < 1 || w.GraphClaims > 1000 || w.Scopes < 1 || w.Scopes > 16 || w.DelayMS < 0 || w.DelayMS > 1000 || w.Processes < 1 || w.Processes > 2 || w.Turns < 1 || w.Turns > 32 || w.BackfillRoots < 0 || w.BackfillRoots > 64 {
		return errors.New("workload exceeds the declared disposable experiment bounds")
	}
	return nil
}

func main() {
	if err := entry(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func entry(args []string) error {
	// The interactive recorder uses normal OS signal termination. Completed
	// records are already synced; unfinished active timing stays only in memory.
	// Installing a context handler here would suppress termination while Scan
	// waits for terminal input, which does not observe a context.
	if len(args) > 0 && args[0] == "review-session" {
		return reviewCommand(args[1:], os.Stdin, os.Stdout)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return command(ctx, args)
}

func command(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: memory-stage4-pilot run|worker|review-session")
	}
	if args[0] == "worker" {
		return workerCommand(ctx, args[1:])
	}
	if args[0] != "run" {
		return errors.New("unknown pilot command")
	}
	f := flag.NewFlagSet("run", flag.ContinueOnError)
	var w workload
	f.StringVar(&w.Mode, "mode", "disabled", "disabled, new, or history")
	f.IntVar(&w.RetainedEvents, "retained-events", 10000, "archived fixture events")
	f.IntVar(&w.SourceBytes, "source-bytes", 256, "exact UTF-8 bytes per foreground input")
	f.IntVar(&w.GraphClaims, "graph-claims", 1, "accepted fixture Claims")
	f.IntVar(&w.Scopes, "scopes", 1, "distinct exact session destinations")
	f.IntVar(&w.DelayMS, "delay-ms", 25, "scripted inference service delay")
	f.IntVar(&w.Processes, "processes", 1, "cooperating worker processes")
	f.IntVar(&w.Turns, "turns", 16, "fixed foreground turns")
	f.IntVar(&w.BackfillRoots, "backfill-roots", 16, "historical roots competing with new work")
	output := f.String("output", "", "new report file; must not already exist")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || *output == "" {
		return errors.New("run requires --output and no positional arguments")
	}
	if err := w.validate(); err != nil {
		return err
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	r, runErr := runWorkload(ctx, w)
	if runErr != nil {
		r.Error = runErr.Error()
	}
	err = json.NewEncoder(file).Encode(r)
	if err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	return runErr
}
