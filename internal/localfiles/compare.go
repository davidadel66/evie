package localfiles

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// Compare (changes.spec.md story 6a): a Change's diff, from where the
// branch left its base to its head, read with the same hardened git as the
// folder view's review. Each file has its status, line counts, the blob it
// ends at (so a "Viewed" mark can reset when the file changes again) and
// its patch's hunks. A file review excludes, such as .env, is listed but
// its diff is withheld.

// ComparedFile is one file of a comparison.
type ComparedFile struct {
	Path string `json:"path"`
	// OldPath is a renamed or copied file's earlier path.
	OldPath string `json:"old_path,omitempty"`
	// Status is added, modified, deleted, renamed or copied.
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
	// Blob is the file's object at the head; empty when it was deleted.
	Blob string `json:"blob,omitempty"`
	// Patch is the file's hunks, from its first "@@" line.
	Patch string `json:"patch,omitempty"`
	// Excluded is a file review withholds, such as a secrets file.
	Excluded bool `json:"excluded,omitempty"`
	// Truncated is a patch too large to show.
	Truncated bool `json:"truncated,omitempty"`
}

// Comparison is the files one revision changes against another.
type Comparison struct {
	Files []ComparedFile `json:"files"`
	// Truncated says some files have no patch because there are too many.
	Truncated bool `json:"truncated,omitempty"`
}

// maxComparedPatches bounds how many files' patches one comparison reads.
const maxComparedPatches = 100

// Compare is what head changes since it left base: git's three-dot
// comparison, renames found.
func Compare(ctx context.Context, folder, base, head string) (Comparison, error) {
	result := Comparison{Files: []ComparedFile{}}
	if !validRevision(base) || !validRevision(head) {
		return result, errors.New("Invalid revision")
	}
	span := base + "..." + head
	raw, err := git(ctx, folder, "diff", "--no-ext-diff", "--no-textconv", "-M", "--raw", "-z", "--no-abbrev", span)
	if err != nil {
		return result, err
	}
	counts, err := git(ctx, folder, "diff", "--no-ext-diff", "--no-textconv", "-M", "--numstat", "-z", span)
	if err != nil {
		return result, err
	}
	numbers := numstat(counts)
	tokens := strings.Split(raw, "\x00")
	for i := 0; i < len(tokens); i++ {
		meta := tokens[i]
		if !strings.HasPrefix(meta, ":") {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 5 || i+1 >= len(tokens) {
			break
		}
		f := ComparedFile{Blob: fields[3], Path: tokens[i+1]}
		i++
		switch code := fields[4]; code[0] {
		case 'A':
			f.Status = "added"
		case 'D':
			f.Status, f.Blob = "deleted", ""
		case 'R', 'C':
			if i+1 >= len(tokens) {
				break
			}
			f.Status, f.OldPath, f.Path = "renamed", f.Path, tokens[i+1]
			if code[0] == 'C' {
				f.Status = "copied"
			}
			i++
		default:
			f.Status = "modified"
		}
		if strings.Trim(f.Blob, "0") == "" {
			f.Blob = ""
		}
		n := numbers[f.Path]
		f.Additions, f.Deletions, f.Binary = n.additions, n.deletions, n.binary
		f.Excluded = !allowed(folder, f.Path) || f.OldPath != "" && !allowed(folder, f.OldPath)
		if f.Excluded {
			f.Additions, f.Deletions = 0, 0
		}
		result.Files = append(result.Files, f)
	}
	read := 0
	for i := range result.Files {
		f := &result.Files[i]
		if f.Excluded || f.Binary {
			continue
		}
		if read == maxComparedPatches {
			result.Truncated = true
			continue
		}
		read++
		paths := []string{f.Path}
		if f.OldPath != "" {
			paths = append(paths, f.OldPath)
		}
		text, err := git(ctx, folder, append([]string{"diff", "--no-ext-diff", "--no-textconv", "-M", "--unified=3", span, "--"}, paths...)...)
		if err != nil {
			// Too large to preview, or unreadable: listed without its hunks.
			f.Truncated = true
			continue
		}
		f.Patch = hunks(text)
	}
	return result, nil
}

type lineCounts struct {
	additions, deletions int
	binary               bool
}

// numstat reads `git diff --numstat -z`: "added\tdeleted\tpath", or for a
// rename "added\tdeleted\t" followed by its old and new paths; a binary
// file counts "-".
func numstat(out string) map[string]lineCounts {
	counts := map[string]lineCounts{}
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); i++ {
		parts := strings.SplitN(tokens[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		path := parts[2]
		if path == "" && i+2 < len(tokens) {
			path = tokens[i+2]
			i += 2
		}
		added, addErr := strconv.Atoi(parts[0])
		deleted, delErr := strconv.Atoi(parts[1])
		counts[path] = lineCounts{additions: added, deletions: deleted, binary: addErr != nil || delErr != nil}
	}
	return counts
}

// hunks is a file's patch from its first hunk header, without git's file
// header lines.
func hunks(patch string) string {
	if strings.HasPrefix(patch, "@@") {
		return patch
	}
	if i := strings.Index(patch, "\n@@"); i >= 0 {
		return patch[i+1:]
	}
	return ""
}

// validRevision is a revision git cannot read as an option or more than
// one argument.
func validRevision(revision string) bool {
	return revision != "" && len(revision) <= 255 && !strings.HasPrefix(revision, "-") && !strings.ContainsAny(revision, " \t\r\n\x00")
}
