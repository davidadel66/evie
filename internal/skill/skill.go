// Package skill checks one skill in the Agent Skills format against Evie's
// rules (cmd/evie/docs/active/skills.spec.md): a SKILL.md with YAML
// frontmatter and Markdown instructions, plus optional text reference
// files, with no scripts, executables or links, only UTF-8 text, and
// within Evie's size limits. A version is identified by the SHA-256 of its
// canonical manifest. Checking never runs or substitutes anything in a
// skill; what Evie will not honour is reported as warnings.
package skill

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	// InstructionsFile is the file every skill has.
	InstructionsFile = "SKILL.md"
	// MaxFileBytes bounds SKILL.md and each reference file.
	MaxFileBytes = 32 << 10
	// MaxFiles bounds the files of one skill.
	MaxFiles = 64
	// MaxTotalBytes bounds one skill's files together.
	MaxTotalBytes = 1 << 20
	// MaxNameLength and MaxDescriptionLength are the format's limits, in
	// characters.
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
	// ManifestFormat tags the canonical manifest a version hash covers; a
	// change to it is a new format and changes every hash.
	ManifestFormat = "evie.skill.manifest/1"
)

// builtinCommands are the composer's own slash commands; a skill of the
// same name would change their meaning.
var builtinCommands = []string{"compact", "model"}

var (
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidHash reports whether h is a version hash: 64 lowercase hex digits.
func ValidHash(h string) bool { return hashPattern.MatchString(h) }

// ValidName reports whether name is a skill name: 1 to 64 lowercase
// letters, digits and hyphens, starting with a letter.
func ValidName(name string) bool { return len(name) <= MaxNameLength && namePattern.MatchString(name) }

// Problem is one reason a skill is refused.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// InvalidError lists every reason a skill is refused.
type InvalidError struct {
	Problems []Problem
}

func (e *InvalidError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Path + ": " + p.Message
	}
	return "invalid skill: " + strings.Join(parts, "; ")
}

// Warning kinds: what a skill asks for that Evie will not do.
const (
	WarningFrontmatter  = "frontmatter"
	WarningShellCommand = "shell-command"
	WarningPlaceholder  = "placeholder"
	WarningToolMention  = "tool-mention"
)

// Warning is one thing a skill uses that behaves differently in Evie.
type Warning struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// ManifestFile is one file of a version.
type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Manifest is a version's canonical file list: sorted relative paths, each
// with its SHA-256 and size.
type Manifest struct {
	Format string         `json:"format"`
	Files  []ManifestFile `json:"files"`
}

// Hash is the version hash: the SHA-256 of the manifest's canonical JSON,
// as 64 lowercase hex digits.
func (m Manifest) Hash() string {
	data, err := json.Marshal(m)
	if err != nil {
		panic(err) // a Manifest always encodes
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Paths are the manifest's file paths, in order.
func (m Manifest) Paths() []string {
	out := make([]string, len(m.Files))
	for i, f := range m.Files {
		out[i] = f.Path
	}
	return out
}

// ManifestOf builds the canonical manifest of files.
func ManifestOf(files map[string][]byte) Manifest {
	m := Manifest{Format: ManifestFormat, Files: []ManifestFile{}}
	for p, data := range files {
		sum := sha256.Sum256(data)
		m.Files = append(m.Files, ManifestFile{Path: p, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))})
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	return m
}

// Metadata is what the frontmatter fields Evie recognises say.
type Metadata struct {
	Description string `json:"description"`
	// ArgumentHint is shown in the composer.
	ArgumentHint string `json:"argument_hint,omitempty"`
	// License is recorded with the version.
	License string `json:"license,omitempty"`
	// DisableModelInvocation makes the skill owner-only: invocable with
	// /name, absent from the model's catalog and tool.
	DisableModelInvocation bool `json:"disable_model_invocation,omitempty"`
}

// Version is one checked skill.
type Version struct {
	Name     string
	Metadata Metadata
	// Instructions is SKILL.md after its frontmatter.
	Instructions string
	Files        map[string][]byte
	Manifest     Manifest
	Hash         string
	Warnings     []Warning
}

// Parse checks a skill's files (slash paths relative to its folder) for a
// folder named name.
func Parse(name string, files map[string][]byte) (Version, error) {
	v, problems := check(name, files, nil)
	if len(problems) > 0 {
		return Version{}, &InvalidError{Problems: problems}
	}
	return v, nil
}

// Refusal messages shared by the folder walker and Parse.
func tooManyFiles() string {
	return fmt.Sprintf("has more than %d files; a skill has at most %d", MaxFiles, MaxFiles)
}
func tooLarge() string {
	return fmt.Sprintf("is more than %d KiB in total; a skill is at most %d KiB", MaxTotalBytes>>10, MaxTotalBytes>>10)
}
func fileTooLarge(size int64) string {
	return fmt.Sprintf("is %d bytes; each file is at most %d KiB", size, MaxFileBytes>>10)
}

const scriptsRefused = "a scripts/ folder is refused: Evie does not run skill scripts"

// pathProblem is why a relative path cannot be part of a skill, or "".
func pathProblem(p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || path.Clean(p) != p {
		return "is not a clean relative path inside the skill"
	}
	for _, c := range p {
		if c < 0x20 || c == 0x7f {
			return "has a control character in its name"
		}
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "." || part == ".." {
			return "is not a clean relative path inside the skill"
		}
		if strings.EqualFold(part, ".git") {
			return "is inside a .git folder, which a skill cannot hold"
		}
	}
	if len(parts) > 1 && parts[0] == "scripts" {
		return scriptsRefused
	}
	return ""
}

// check checks files for a folder named name; refused names paths already
// refused before they were read.
func check(name string, files map[string][]byte, refused map[string]bool) (Version, []Problem) {
	var problems []Problem
	add := func(p, format string, args ...any) {
		problems = append(problems, Problem{Path: p, Message: fmt.Sprintf(format, args...)})
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) > MaxFiles {
		add(".", "%s", tooManyFiles())
	}
	var total int64
	for _, p := range paths {
		data := files[p]
		total += int64(len(data))
		if msg := pathProblem(p); msg != "" {
			add(p, "%s", msg)
			continue
		}
		switch {
		case len(data) > MaxFileBytes:
			add(p, "%s", fileTooLarge(int64(len(data))))
		case bytes.IndexByte(data, 0) >= 0:
			add(p, "contains a NUL byte; a skill holds only text")
		case !utf8.Valid(data):
			add(p, "is not UTF-8 text; a skill holds only text")
		}
	}
	if total > MaxTotalBytes {
		add(".", "%s", tooLarge())
	}
	v := Version{Name: name, Files: files}
	text, ok := files[InstructionsFile]
	if !ok {
		if !refused[InstructionsFile] {
			add(InstructionsFile, "is missing")
		}
		return v, problems
	}
	if len(text) > MaxFileBytes || !utf8.Valid(text) {
		return v, problems
	}
	front, body, frontProblems := frontmatter(name, string(text))
	problems = append(problems, frontProblems...)
	if len(problems) > 0 {
		return v, problems
	}
	v.Metadata, v.Instructions = front.metadata, body
	v.Manifest = ManifestOf(files)
	v.Hash = v.Manifest.Hash()
	v.Warnings = append(front.warnings, bodyWarnings(body, files)...)
	return v, nil
}

type parsedFront struct {
	metadata Metadata
	warnings []Warning
}

// frontmatter reads SKILL.md's frontmatter: a YAML mapping between a first
// line of --- and the next line of ---.
func frontmatter(folder, text string) (parsedFront, string, []Problem) {
	var out parsedFront
	var problems []Problem
	add := func(format string, args ...any) {
		problems = append(problems, Problem{Path: InstructionsFile, Message: fmt.Sprintf(format, args...)})
	}
	first, rest, found := strings.Cut(text, "\n")
	if !found || strings.TrimRight(first, " \t\r") != "---" {
		add("must start with YAML frontmatter between two --- lines")
		return out, "", problems
	}
	var front strings.Builder
	body, closed := "", false
	for rest != "" {
		var line string
		line, rest, _ = strings.Cut(rest, "\n")
		if strings.TrimRight(line, " \t\r") == "---" {
			body, closed = rest, true
			break
		}
		front.WriteString(line + "\n")
	}
	if !closed {
		add("frontmatter has no closing --- line")
		return out, "", problems
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front.String()), &doc); err != nil {
		add("frontmatter is not valid YAML: %v", err)
		return out, "", problems
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		add("frontmatter must be a YAML mapping with name and description")
		return out, "", problems
	}
	fields := doc.Content[0].Content
	seen := map[string]bool{}
	var name, description *yaml.Node
	for i := 0; i+1 < len(fields); i += 2 {
		key, value := fields[i], fields[i+1]
		if key.Kind != yaml.ScalarNode {
			add("frontmatter line %d: a key must be text", key.Line)
			continue
		}
		if seen[key.Value] {
			add("frontmatter line %d: %s is given twice", key.Line, key.Value)
			continue
		}
		seen[key.Value] = true
		text := value.Kind == yaml.ScalarNode && value.Tag == "!!str"
		switch key.Value {
		case "name":
			name = value
		case "description":
			description = value
		case "disable-model-invocation":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				add("disable-model-invocation must be true or false")
				continue
			}
			out.metadata.DisableModelInvocation = value.Value == "true"
		case "argument-hint", "license":
			if !text {
				out.warnings = append(out.warnings, Warning{Kind: WarningFrontmatter,
					Message: fmt.Sprintf("SKILL.md frontmatter field %q is not text and is ignored (quote it to keep it)", key.Value)})
				continue
			}
			if key.Value == "license" {
				out.metadata.License = value.Value
			} else {
				out.metadata.ArgumentHint = value.Value
			}
		default:
			out.warnings = append(out.warnings, Warning{Kind: WarningFrontmatter,
				Message: fmt.Sprintf("SKILL.md frontmatter field %q is kept but has no effect in Evie", key.Value)})
		}
	}
	switch {
	case name == nil:
		add("name is missing")
	case name.Kind != yaml.ScalarNode || name.Tag != "!!str":
		add("name must be text")
	case !ValidName(name.Value):
		add("name %q must be 1 to %d lowercase letters, digits and hyphens, starting with a letter", name.Value, MaxNameLength)
	case name.Value != folder:
		add("name %q does not match the folder name %q", name.Value, folder)
	default:
		for _, command := range builtinCommands {
			if name.Value == command {
				add("name %q clashes with Evie's built-in /%s command", name.Value, command)
			}
		}
	}
	switch {
	case description == nil:
		add("description is missing")
	case description.Kind != yaml.ScalarNode || description.Tag != "!!str":
		add("description must be text")
	case strings.TrimSpace(description.Value) == "":
		add("description is empty")
	case utf8.RuneCountInString(description.Value) > MaxDescriptionLength:
		add("description is %d characters; at most %d", utf8.RuneCountInString(description.Value), MaxDescriptionLength)
	default:
		out.metadata.Description = description.Value
	}
	return out, body, problems
}

var (
	// shellCommand is Claude Code's inline shell command, !`command`.
	shellCommand = regexp.MustCompile("!`[^`\n]+`")
	// placeholder is a Claude Code argument or session placeholder.
	placeholder = regexp.MustCompile(`\$ARGUMENTS(?:\[[0-9]+\])?|\$\{CLAUDE_[A-Z_]+\}`)
	// claudeTool is a mention of a Claude Code tool: a distinctive name
	// anywhere, or a common word written as a tool (`Read`, the Read tool,
	// Read(...)).
	claudeTool = regexp.MustCompile("\\b(TodoWrite|WebFetch|WebSearch|NotebookEdit|MultiEdit|AskUserQuestion|ExitPlanMode|Bash)\\b" +
		"|`(Read|Edit|Write|Agent|Task|Skill|Glob|Grep)`" +
		"|\\b(Read|Edit|Write|Agent|Task|Skill|Glob|Grep)(?: tool\\b|\\()")
)

const maxListed = 5

// occurrences lists up to maxListed matches of re in text with their line
// numbers.
func occurrences(re *regexp.Regexp, text string, lineOffset int) []string {
	var out []string
	for i, line := range strings.Split(text, "\n") {
		for _, m := range re.FindAllString(line, -1) {
			out = append(out, fmt.Sprintf("%s (line %d)", m, lineOffset+i+1))
		}
	}
	return out
}

func listed(items []string) string {
	if len(items) <= maxListed {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:maxListed], ", ") + fmt.Sprintf(" and %d more", len(items)-maxListed)
}

func bodyWarnings(body string, files map[string][]byte) []Warning {
	var out []Warning
	offset := strings.Count(string(files[InstructionsFile]), "\n") - strings.Count(body, "\n")
	if found := occurrences(shellCommand, body, offset); len(found) > 0 {
		out = append(out, Warning{Kind: WarningShellCommand,
			Message: "SKILL.md has inline shell commands, which Evie never runs: " + listed(found)})
	}
	if found := occurrences(placeholder, body, offset); len(found) > 0 {
		out = append(out, Warning{Kind: WarningPlaceholder,
			Message: "SKILL.md has argument placeholders, which Evie never substitutes: " + listed(found)})
	}
	tools, where := map[string]bool{}, []string{}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		text := string(files[p])
		if p == InstructionsFile {
			text = body
		}
		found := false
		for _, m := range claudeTool.FindAllStringSubmatch(text, -1) {
			for _, name := range m[1:] {
				if name != "" {
					tools[name], found = true, true
				}
			}
		}
		if found {
			where = append(where, p)
		}
	}
	if len(tools) > 0 {
		names := make([]string, 0, len(tools))
		for name := range tools {
			names = append(names, name)
		}
		sort.Strings(names)
		out = append(out, Warning{Kind: WarningToolMention,
			Message: fmt.Sprintf("mentions Claude Code tools (%s in %s); Evie's tools differ", strings.Join(names, ", "), listed(where))})
	}
	return out
}

// Source reads a skill's files by slash path relative to its folder, as
// procedural.Source does at one commit.
type Source interface {
	ReadFile(name string) ([]byte, error)
}

// ReadVersion reads the files m lists from src, checks them for a folder
// named name, and requires them to hash to m, so a version read back from
// where it was stored is exactly the one recorded.
func ReadVersion(name string, src Source, m Manifest) (Version, error) {
	files := map[string][]byte{}
	for _, f := range m.Files {
		data, err := src.ReadFile(f.Path)
		if err != nil {
			return Version{}, err
		}
		files[f.Path] = data
	}
	v, err := Parse(name, files)
	if err != nil {
		return Version{}, err
	}
	if want := m.Hash(); v.Hash != want {
		return Version{}, fmt.Errorf("skill %s reads back with version hash %s, not %s", name, v.Hash, want)
	}
	return v, nil
}
