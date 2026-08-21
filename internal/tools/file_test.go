package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyEdit(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		old      string
		new      string
		wantOut  string
		wantLine int
		wantErr  bool
	}{
		{
			name:     "replaces single match",
			content:  "My name is David\n",
			old:      "David",
			new:      "Kaylee",
			wantOut:  "My name is Kaylee\n",
			wantLine: 1,
		},
		{
			name:     "reports the line the match starts on",
			content:  "alpha\nbeta\ngamma\n",
			old:      "gamma",
			new:      "delta",
			wantOut:  "alpha\nbeta\ndelta\n",
			wantLine: 3,
		},
		{
			name:    "empty old string is an error",
			content: "anything",
			old:     "",
			new:     "x",
			wantErr: true,
		},
		{
			name:    "no match is an error",
			content: "alpha\nbeta\n",
			old:     "zeta",
			new:     "x",
			wantErr: true,
		},
		{
			name:    "multiple matches is an error",
			content: "dup\ndup\n",
			old:     "dup",
			new:     "x",
			wantErr: true,
		},
		{
			name:    "old equals new is an error",
			content: "alpha",
			old:     "alpha",
			new:     "alpha",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, line, err := applyEdit(tt.content, tt.old, tt.new)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("applyEdit(%q, %q, %q) = no error, want an error", tt.content, tt.old, tt.new)
				}
				return
			}

			if err != nil {
				t.Fatalf("applyEdit(%q, %q, %q) returned error: %v", tt.content, tt.old, tt.new, err)
			}
			if out != tt.wantOut {
				t.Errorf("content = %q, want %q", out, tt.wantOut)
			}
			if line != tt.wantLine {
				t.Errorf("line = %d, want %d", line, tt.wantLine)
			}
		})
	}
}

func TestStripLineNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "every line prefixed is stripped",
			in:   "     1\tpackage tools\n     2\t\n     3\tfunc main() {",
			want: "package tools\n\nfunc main() {",
		},
		{
			name: "no padding still counts as a prefix",
			in:   "1\talpha\n2\tbeta",
			want: "alpha\nbeta",
		},
		{
			name: "indentation after the tab is preserved",
			in:   "     1\tfunc f() {\n     2\t\treturn 1\n     3\t}",
			want: "func f() {\n\treturn 1\n}",
		},
		{
			name: "one unprefixed line leaves everything untouched",
			in:   "     1\talpha\nbeta\n     3\tgamma",
			want: "     1\talpha\nbeta\n     3\tgamma",
		},
		{
			name: "ordinary source is untouched",
			in:   "func main() {\n\tfmt.Println(1)\n}",
			want: "func main() {\n\tfmt.Println(1)\n}",
		},
		{
			name: "single prefixed line",
			in:   "    42\tfoo := bar",
			want: "foo := bar",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "digits without a tab are not a prefix",
			in:   "     1 alpha\n     2 beta",
			want: "     1 alpha\n     2 beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripLineNumbers(tt.in); got != tt.want {
				t.Errorf("stripLineNumbers(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() failed: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() failed: %v", err)
	}

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		// Allowed.
		{name: "absolute path passes through", in: "/tmp/notes.md", want: "/tmp/notes.md"},
		{name: "tilde expands to home", in: "~/notes.md", want: filepath.Join(home, "notes.md")},
		{name: "bare tilde is home", in: "~", want: home},
		{name: "relative path resolves against cwd", in: "notes.md", want: filepath.Join(cwd, "notes.md")},
		{name: "surrounding whitespace is trimmed", in: "  /tmp/notes.md  ", want: "/tmp/notes.md"},
		{name: "harmless traversal is cleaned", in: "/tmp/a/../notes.md", want: "/tmp/notes.md"},
		{name: "near miss on a denied dir is allowed", in: "~/.financeXYZ/notes.md", want: filepath.Join(home, ".financeXYZ", "notes.md")},
		{name: "finance dir is deliberately open", in: "~/.finance/merchantLookup.json", want: filepath.Join(home, ".finance", "merchantLookup.json")},
		{name: "file merely containing env in its name is allowed", in: "/tmp/environment.md", want: "/tmp/environment.md"},

		// Allowed near-miss.
		{
			name: "evie database near-miss is allowed",
			in:   filepath.Join(home, ".evie", "evie.db.backup"),
			want: filepath.Join(home, ".evie", "evie.db.backup"),
		},

		// Rejected: memory-owned storage.
		{name: "evie database is fenced", in: filepath.Join(home, ".evie", "evie.db"), wantErr: true},
		{name: "evie WAL is fenced", in: filepath.Join(home, ".evie", "evie.db-wal"), wantErr: true},
		{name: "evie shared memory is fenced", in: filepath.Join(home, ".evie", "evie.db-shm"), wantErr: true},
		{name: "procedural root is fenced", in: filepath.Join(home, ".evie", "procedural"), wantErr: true},
		{name: "procedural descendants are fenced", in: filepath.Join(home, ".evie", "procedural", "system", "user.md"), wantErr: true},

		// Rejected: bad input.
		{name: "empty path errors", in: "", wantErr: true},
		{name: "whitespace-only path errors", in: "   ", wantErr: true},
		{name: "~user is rejected rather than guessed", in: "~alice/notes.md", wantErr: true},

		// Rejected: denied directories.
		{name: "the denied dir itself is fenced", in: "~/.ssh", wantErr: true},
		{name: "aws credentials are fenced", in: "~/.aws/credentials", wantErr: true},
		{name: "gnupg is fenced", in: "~/.gnupg/secring.gpg", wantErr: true},
		{name: "traversal into a denied dir is caught after cleaning", in: "~/projects/../.ssh/id_rsa", wantErr: true},

		// Rejected: denied file names.
		{name: "dotenv is fenced", in: "/tmp/project/.env", wantErr: true},
		{name: "dotenv variants are fenced", in: "/tmp/project/.env.production", wantErr: true},
		{name: "zshrc is fenced", in: "~/.zshrc", wantErr: true},
		{name: "netrc is fenced anywhere", in: "/tmp/.netrc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePath(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolvePath(%q) = %q, want an error", tt.in, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolvePath(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNumbered(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "multi-line content",
			in:   "package tools\n\nfunc f() {}",
			want: "     1\tpackage tools\n     2\t\n     3\tfunc f() {}\n",
		},
		{
			name: "single line without a trailing newline",
			in:   "alpha",
			want: "     1\talpha\n",
		},
		{
			name: "trailing newline yields a numbered empty last line",
			in:   "alpha\n",
			want: "     1\talpha\n     2\t\n",
		},
		{
			name: "empty content is one numbered blank line",
			in:   "",
			want: "     1\t\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := numbered(tt.in); got != tt.want {
				t.Errorf("numbered(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// numbered and stripLineNumbers are inverses: anything numbered must strip
// back to exactly what went in. This is the property edit_file depends on
// when the model quotes back what read_file showed it.
func TestNumberedRoundTrips(t *testing.T) {
	inputs := []string{
		"package tools\n\nfunc f() {\n\treturn\n}",
		"alpha",
		"a\nb\nc",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			// numbered always appends a trailing newline, which splits into a
			// final empty line the original didn't have; drop it before
			// comparing.
			got := strings.TrimSuffix(stripLineNumbers(strings.TrimSuffix(numbered(in), "\n")), "\n")
			if got != in {
				t.Errorf("round trip of %q = %q", in, got)
			}
		})
	}
}

func TestEditFile(t *testing.T) {
	// Write a real file per subtest, edit it through the tool, and read the
	// bytes back — the disk is the only witness that matters here.
	setup := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "notes.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("setup write failed: %v", err)
		}
		return path
	}

	call := func(path, old, new string) (string, error) {
		args, _ := json.Marshal(map[string]string{
			"path": path, "old_string": old, "new_string": new,
		})
		return editFile(string(args))
	}

	t.Run("replaces a unique match on disk", func(t *testing.T) {
		path := setup(t, "alpha\nbeta\ngamma\n")

		got, err := call(path, "beta", "delta")
		if err != nil {
			t.Fatalf("editFile returned error: %v", err)
		}
		if !strings.Contains(got, "line 2") {
			t.Errorf("result %q does not report line 2", got)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back failed: %v", err)
		}
		if want := "alpha\ndelta\ngamma\n"; string(data) != want {
			t.Errorf("file = %q, want %q", string(data), want)
		}
	})

	t.Run("strips line-number prefixes the model echoed back", func(t *testing.T) {
		path := setup(t, "func f() {\n\treturn 1\n}\n")

		if _, err := call(path, "     2\t\treturn 1", "     2\t\treturn 2"); err != nil {
			t.Fatalf("editFile returned error: %v", err)
		}

		data, _ := os.ReadFile(path)
		if want := "func f() {\n\treturn 2\n}\n"; string(data) != want {
			t.Errorf("file = %q, want %q", string(data), want)
		}
	})

	t.Run("ambiguous match leaves the file untouched", func(t *testing.T) {
		const content = "dup\ndup\n"
		path := setup(t, content)

		if _, err := call(path, "dup", "x"); err == nil {
			t.Fatal("editFile succeeded on an ambiguous match, want an error")
		}

		data, _ := os.ReadFile(path)
		if string(data) != content {
			t.Errorf("file was modified on a failed edit: %q", string(data))
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.md")
		if _, err := call(path, "a", "b"); err == nil {
			t.Fatal("editFile succeeded on a missing file, want an error")
		}
	})

	t.Run("directory errors", func(t *testing.T) {
		if _, err := call(t.TempDir(), "a", "b"); err == nil {
			t.Fatal("editFile succeeded on a directory, want an error")
		}
	})

	t.Run("fenced path errors", func(t *testing.T) {
		if _, err := call("~/.ssh/config", "a", "b"); err == nil {
			t.Fatal("editFile succeeded on a fenced path, want an error")
		}
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		path := setup(t, "alpha\n")
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatalf("chmod failed: %v", err)
		}

		if _, err := call(path, "alpha", "beta"); err != nil {
			t.Fatalf("editFile returned error: %v", err)
		}

		info, _ := os.Stat(path)
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("perm = %o, want 600", got)
		}
	})
}

func TestResolvePathRejectsSymlinksToMemoryStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	targets := []struct {
		name string
		path string
	}{
		{
			name: "database",
			path: filepath.Join(home, ".evie", "evie.db"),
		},
		{
			name: "procedural file",
			path: filepath.Join(home, ".evie", "procedural", "system", "user.md"),
		},
	}

	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(tt.path), 0o700); err != nil {
				t.Fatalf("create target directory: %v", err)
			}
			if err := os.WriteFile(tt.path, []byte("private"), 0o0600); err != nil {
				t.Fatalf("create target: %v", err)
			}

			link := filepath.Join(t.TempDir(), "innocent-looking-file")
			if err := os.Symlink(tt.path, link); err != nil {
				t.Fatalf("create symlink: %v", err)
			}
			if got, err := resolvePath(link); err == nil {
				t.Fatalf("resolvePath(%q) = %q, want an error", link, got)
			}
		})
	}
}

func TestPreparedFileEditRejectsStalePreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	const before = "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	args, _ := json.Marshal(map[string]string{
		"path": path, "old_string": "beta", "new_string": "delta",
	})

	prepared, err := prepareEditFileTool(string(args))
	if err != nil {
		t.Fatalf("prepareEditFileTool returned error: %v", err)
	}
	preview := prepared.Preview
	if preview == nil {
		t.Fatal("prepared preview is nil")
	}
	if preview.Path != path || preview.OldText != before || preview.NewText != "alpha\ndelta\ngamma\n" || preview.IsNew {
		t.Errorf("preview = %+v", preview)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if string(data) != before {
		t.Errorf("preview modified the file: %q", data)
	}

	const concurrent = "alpha\nbeta\ngamma\nexternal change\n"
	if err := os.WriteFile(path, []byte(concurrent), 0o644); err != nil {
		t.Fatalf("concurrent write failed: %v", err)
	}
	if _, err := prepared.Execute(); err == nil || !strings.Contains(err.Error(), "changed after the approval preview") {
		t.Fatalf("prepared execute error = %v, want stale-preview error", err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != concurrent {
		t.Errorf("stale prepared edit overwrote concurrent bytes: %q", data)
	}

	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("reset write failed: %v", err)
	}
	prepared, err = prepareEditFileTool(string(args))
	if err != nil {
		t.Fatalf("second prepare failed: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("concurrent chmod failed: %v", err)
	}
	if _, err := prepared.Execute(); err == nil || !strings.Contains(err.Error(), "permissions changed after the approval preview") {
		t.Fatalf("prepared execute error = %v, want stale-permissions error", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("stale prepared edit restored old permissions: %o", info.Mode().Perm())
	}
}

func TestWriteFileAtomicLeavesNoTempFiles(t *testing.T) {
	// The temp file lives in the target's own directory, so a leaked one is
	// litter in the user's working tree — assert it never survives.
	strays := func(t *testing.T, dir string) []string {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		var found []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".evie-") {
				found = append(found, e.Name())
			}
		}
		return found
	}

	call := func(path, old, new string) (string, error) {
		args, _ := json.Marshal(map[string]string{
			"path": path, "old_string": old, "new_string": new,
		})
		return editFile(string(args))
	}

	t.Run("after a successful edit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "notes.md")
		if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if _, err := call(path, "alpha", "beta"); err != nil {
			t.Fatalf("editFile: %v", err)
		}
		if got := strays(t, dir); got != nil {
			t.Errorf("leftover temp files: %v", got)
		}
	})

	t.Run("after a failed edit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "notes.md")
		if err := os.WriteFile(path, []byte("dup\ndup\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if _, err := call(path, "dup", "x"); err == nil {
			t.Fatal("editFile succeeded on an ambiguous match")
		}
		if got := strays(t, dir); got != nil {
			t.Errorf("leftover temp files: %v", got)
		}
	})

	t.Run("content and perms survive the rename", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "notes.md")
		if err := os.WriteFile(path, []byte("alpha\n"), 0o600); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := writeFileAtomic(path, []byte("beta\n"), 0o600); err != nil {
			t.Fatalf("writeFileAtomic: %v", err)
		}

		data, _ := os.ReadFile(path)
		if string(data) != "beta\n" {
			t.Errorf("content = %q, want %q", string(data), "beta\n")
		}
		info, _ := os.Stat(path)
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("perm = %o, want 600", got)
		}
		if got := strays(t, dir); got != nil {
			t.Errorf("leftover temp files: %v", got)
		}
	})
}
