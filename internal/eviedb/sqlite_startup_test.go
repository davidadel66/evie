package eviedb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type startupSQLiteError int

func (e startupSQLiteError) Error() string { return fmt.Sprintf("fixture SQLite error %d", e) }
func (e startupSQLiteError) Code() int     { return int(e) }

func TestStage4StartupRetryBoundaries(t *testing.T) {
	t.Run("only typed BUSY retries", func(t *testing.T) {
		calls := 0
		err := connectSQLiteStartupWithin(context.Background(), func(ctx context.Context) error {
			calls++
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("missing connection deadline")
			}
			switch calls {
			case 1:
				return fmt.Errorf("primary: %w", startupSQLiteError(5))
			case 2:
				return fmt.Errorf("extended: %w", startupSQLiteError(517))
			default:
				return nil
			}
		}, time.Second, time.Millisecond)
		if err != nil || calls != 3 {
			t.Fatalf("retry result %d %v", calls, err)
		}
		for _, cause := range []error{startupSQLiteError(6), startupSQLiteError(10), errors.New("database is locked (5) (SQLITE_BUSY)")} {
			calls = 0
			err = connectSQLiteStartupWithin(context.Background(), func(context.Context) error { calls++; return cause }, time.Second, time.Millisecond)
			if err != cause || calls != 1 {
				t.Fatalf("non-BUSY retry %d %v", calls, err)
			}
		}
	})
	t.Run("cancelled before connection", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := connectSQLiteStartupWithin(ctx, func(context.Context) error { calls++; return nil }, time.Second, time.Millisecond)
		if !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatalf("cancelled attempt %d %v", calls, err)
		}
	})
	t.Run("cancelled between attempts retains code", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		err := connectSQLiteStartupWithin(ctx, func(context.Context) error { calls++; cancel(); return startupSQLiteError(5) }, time.Second, time.Second)
		if !errors.Is(err, context.Canceled) || !errors.Is(err, startupSQLiteError(5)) || calls != 1 {
			t.Fatalf("cancel retry %d %v", calls, err)
		}
	})
	t.Run("retry deadline preserves last BUSY", func(t *testing.T) {
		calls := 0
		start := time.Now()
		err := connectSQLiteStartupWithin(context.Background(), func(context.Context) error { calls++; return startupSQLiteError(517) }, 10*time.Millisecond, time.Second)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, startupSQLiteError(517)) || calls != 1 || time.Since(start) > time.Second {
			t.Fatalf("unbounded retry %d %v", calls, err)
		}
	})
	t.Run("late successful connection cannot erase cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := connectSQLiteStartupWithin(ctx, func(context.Context) error { cancel(); return nil }, time.Second, time.Millisecond)
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

type stage4StartupReceipt struct {
	OK            bool   `json:"ok"`
	Journal       string `json:"journal"`
	ForeignKeys   int    `json:"foreign_keys"`
	BusyTimeout   int    `json:"busy_timeout"`
	SQLiteVersion string `json:"sqlite_version"`
	Error         string `json:"error,omitempty"`
}

// Each child enters the ordinary public path with a fresh lazy pool. The stdin
// barrier synchronizes independent processes without preopening a connection or
// pre-enabling WAL. Both a new database and its retained reopen are exercised.
func TestStage4StartupIndependentProcesses(t *testing.T) {
	if path := os.Getenv("EVIE_STAGE4_STARTUP_CHILD"); path != "" {
		var one [1]byte
		if _, err := io.ReadFull(os.Stdin, one[:]); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		out := stage4StartupReceipt{}
		db, err := OpenDBAtContext(ctx, path)
		if err == nil {
			err = db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&out.Journal)
			if err == nil {
				err = db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&out.ForeignKeys)
			}
			if err == nil {
				err = db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&out.BusyTimeout)
			}
			if err == nil {
				err = db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&out.SQLiteVersion)
			}
			closeErr := db.Close()
			if err == nil {
				err = closeErr
			}
		}
		out.OK = err == nil
		if err != nil {
			out.Error = err.Error()
		}
		if err = json.NewEncoder(os.Stdout).Encode(out); err != nil {
			os.Exit(2)
		}
		if !out.OK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sqliteVersion := ""
	for wave := 0; wave < 8; wave++ {
		path := filepath.Join(t.TempDir(), "conformance.db")
		for _, state := range []string{"fresh", "existing"} {
			t.Run(fmt.Sprintf("%s_%d", state, wave), func(t *testing.T) {
				type child struct {
					command *exec.Cmd
					input   io.WriteCloser
					output  bytes.Buffer
				}
				children := make([]*child, 3)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				for i := range children {
					c := &child{}
					c.command = exec.CommandContext(ctx, executable, "-test.run=^TestStage4StartupIndependentProcesses$")
					c.command.Env = append(os.Environ(), "EVIE_STAGE4_STARTUP_CHILD="+path, "GORACE=atexit_sleep_ms=0")
					c.command.Stdout = &c.output
					c.command.Stderr = &c.output
					c.input, err = c.command.StdinPipe()
					if err != nil {
						t.Fatal(err)
					}
					if err = c.command.Start(); err != nil {
						t.Fatal(err)
					}
					children[i] = c
				}
				var wg sync.WaitGroup
				for _, c := range children {
					wg.Add(1)
					go func(c *child) { defer wg.Done(); _, _ = c.input.Write([]byte{1}); _ = c.input.Close() }(c)
				}
				wg.Wait()
				for _, c := range children {
					if err = c.command.Wait(); err != nil {
						t.Fatalf("public startup %v: %s", err, c.output.String())
					}
					var result stage4StartupReceipt
					if err = json.Unmarshal(c.output.Bytes(), &result); err != nil {
						t.Fatalf("child receipt %v: %s", err, c.output.String())
					}
					if !result.OK || result.Journal != "wal" || result.ForeignKeys != 1 || result.BusyTimeout != 5000 || result.SQLiteVersion == "" {
						t.Fatalf("connection configuration %+v", result)
					}
					sqliteVersion = result.SQLiteVersion
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatalf("file protection %v %v", info, err)
				}
			})
		}
	}
	receipt, err := json.Marshal(map[string]any{"version": "memory-stage-4-conformance-v1", "scenario": "public_sqlite_startup", "details": map[string]any{"independent_starts": 48, "fresh_and_existing": true, "unprimed_connections": true, "sqlite_version": sqliteVersion, "journal_mode": "wal", "foreign_keys": 1, "busy_timeout_ms": 5000, "file_mode": "0600"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("STAGE4_EVIDENCE " + string(receipt))
}

func TestStage4StartupIncompatibleSchemaFailsWithoutRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incompatible.db")
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE memory_compiler_position_counter(unsupported TEXT);INSERT INTO memory_compiler_position_counter VALUES('preserve')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenDBAt(path)
	if opened != nil {
		opened.Close()
		t.Fatal("incompatible durable schema opened")
	}
	if err == nil || !strings.Contains(err.Error(), "create memory compiler schema") {
		t.Fatalf("application failure lost: %v", err)
	}
	db, err = sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err = db.QueryRow(`SELECT unsupported FROM memory_compiler_position_counter`).Scan(&marker); err != nil || marker != "preserve" {
		t.Fatal(marker, err)
	}
}
