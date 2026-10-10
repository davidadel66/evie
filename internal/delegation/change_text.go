package delegation

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ChangeText is the parent's title and description of a build's Change,
// given with its merge proposal (changes.spec.md story 4a, 2026-10-10). It
// is the Change's title and notes, and in a GitHub Repository its pull
// request's title and description. Proposals from chats pinned before it
// existed carry none.
type ChangeText struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

const (
	// MaxChangeTitleRunes bounds a title as the harness bounds its commit
	// subjects.
	MaxChangeTitleRunes = 72
	// MaxChangeDescriptionBytes bounds a description to a page.
	MaxChangeDescriptionBytes = 4096
)

// Check refuses a title that is not one printable line of at most 72
// characters, and a description that is empty, longer than 4096 bytes or
// holds control characters other than tabs and newlines. Empty text is an
// older chat's proposal and passes.
func (t ChangeText) Check() error {
	if t == (ChangeText{}) {
		return nil
	}
	title, description := strings.TrimSpace(t.Title), strings.TrimSpace(t.Description)
	switch {
	case title == "":
		return errors.New("title is blank: give the Change a one-line title")
	case strings.ContainsAny(title, "\r\n") || !printable(title, ""):
		return errors.New("title must be one line of printable text")
	case utf8.RuneCountInString(title) > MaxChangeTitleRunes:
		return fmt.Errorf("title is limited to %d characters", MaxChangeTitleRunes)
	case description == "":
		return errors.New("description is blank: say what the Change does and why, and how it was checked")
	case len(description) > MaxChangeDescriptionBytes:
		return fmt.Errorf("description is limited to %d bytes", MaxChangeDescriptionBytes)
	case !printable(description, "\t\n\r"):
		return errors.New("description holds control characters other than tabs and newlines")
	}
	return nil
}

func printable(text, allowed string) bool {
	for _, r := range text {
		if unicode.IsControl(r) && !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

// LocalPath is the first of paths the title or description names, or "":
// a pull request is read on GitHub, where the owner's folders mean nothing
// and give away the owner's home folder name. The root folder and empty
// paths are skipped.
func (t ChangeText) LocalPath(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimRight(path, "/")
		if path == "" {
			continue
		}
		if strings.Contains(t.Title, path) || strings.Contains(t.Description, path) {
			return path
		}
	}
	return ""
}
