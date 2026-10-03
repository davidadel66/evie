package tools

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// The explore worker's command lines are parsed here, not by a shell. The
// grammar is the read-only subset of POSIX shell: words, single and double
// quotes, backslash escapes, unquoted globs, and pipelines joined by &&, ||
// and ;. Every other shell construct is refused outright rather than
// interpreted: redirections, $ in any form, backticks, ( ), { }, &, comments,
// ~, newlines and control characters.

// shellWord is one parsed word. value is the literal text after quote
// removal; when glob is set, pattern is the same text as a filepath.Match
// pattern in which only the unquoted *, ? and [ are special.
type shellWord struct {
	value   string
	pattern string
	glob    bool
}

// shellLine is a list of pipelines; ops[i] joins pipelines i and i+1.
type shellLine struct {
	pipelines [][][]shellWord
	ops       []string
}

type wordBuilder struct {
	value, pattern strings.Builder
	glob           bool
}

func (w *wordBuilder) unquoted(c byte) {
	w.value.WriteByte(c)
	w.pattern.WriteByte(c)
	if c == '*' || c == '?' || c == '[' {
		w.glob = true
	}
}

func (w *wordBuilder) quoted(c byte) {
	w.value.WriteByte(c)
	if strings.IndexByte(`*?[]\`, c) >= 0 {
		w.pattern.WriteByte('\\')
	}
	w.pattern.WriteByte(c)
}

func (w *wordBuilder) done() shellWord {
	return shellWord{value: w.value.String(), pattern: w.pattern.String(), glob: w.glob}
}

func controlByte(c byte) bool { return (c < 0x20 && c != '\t') || c == 0x7f }

func parseShellLine(src string) (shellLine, error) {
	var line shellLine
	var pipeline [][]shellWord
	var command []shellWord
	var word *wordBuilder
	start := func() *wordBuilder {
		if word == nil {
			word = &wordBuilder{}
		}
		return word
	}
	endWord := func() {
		if word != nil {
			command = append(command, word.done())
			word = nil
		}
	}
	endCommand := func(op string) error {
		endWord()
		if len(command) == 0 {
			return fmt.Errorf("operator %s has no command before it", op)
		}
		pipeline = append(pipeline, command)
		command = nil
		return nil
	}
	endPipeline := func(op string) error {
		if err := endCommand(op); err != nil {
			return err
		}
		line.pipelines = append(line.pipelines, pipeline)
		line.ops = append(line.ops, op)
		pipeline = nil
		return nil
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		next := byte(0)
		if i+1 < len(src) {
			next = src[i+1]
		}
		switch {
		case c == ' ' || c == '\t':
			endWord()
		case c == '\n' || c == '\r':
			return line, errors.New("a newline ends the line here; run one line and join commands with ; or &&")
		case controlByte(c):
			return line, fmt.Errorf("control character %q is not allowed", c)
		case c == '|' && next == '|':
			if err := endPipeline("||"); err != nil {
				return line, err
			}
			i++
		case c == '|' && next == '&':
			return line, errors.New("|& is not allowed: & starts background jobs and redirections")
		case c == '|':
			if err := endCommand("|"); err != nil {
				return line, err
			}
		case c == '&' && next == '&':
			if err := endPipeline("&&"); err != nil {
				return line, err
			}
			i++
		case c == '&':
			return line, errors.New("& is not allowed: background jobs and &> redirections have no read-only use")
		case c == ';':
			if err := endPipeline(";"); err != nil {
				return line, err
			}
		case c == '<' || c == '>':
			return line, errors.New("redirection (< or >) is not allowed: workers cannot write files, and commands read the paths they are given")
		case c == '(' || c == ')':
			return line, errors.New("subshells and grouping with ( ) are not allowed; quote a literal parenthesis")
		case c == '{' || c == '}':
			return line, errors.New("{ and } (grouping or brace expansion) are not allowed; quote them, as in 'HEAD@{1}'")
		case c == '$':
			return line, errors.New("$ expansion (variables and $(...) command substitution) is not allowed; single-quote a literal $")
		case c == '`':
			return line, errors.New("backtick command substitution is not allowed")
		case c == '\'':
			end := strings.IndexByte(src[i+1:], '\'')
			if end < 0 {
				return line, errors.New("unterminated single quote")
			}
			w := start()
			for _, q := range []byte(src[i+1 : i+1+end]) {
				if controlByte(q) {
					return line, fmt.Errorf("control character %q is not allowed", q)
				}
				w.quoted(q)
			}
			i += end + 1
		case c == '"':
			w := start()
			j := i + 1
			for ; j < len(src) && src[j] != '"'; j++ {
				d := src[j]
				switch {
				case d == '$':
					return line, errors.New("$ expansion (variables and $(...) command substitution) is not allowed, even in double quotes; single-quote a literal $")
				case d == '`':
					return line, errors.New("backtick command substitution is not allowed, even in double quotes")
				case controlByte(d):
					return line, fmt.Errorf("control character %q is not allowed", d)
				case d == '\\' && j+1 < len(src) && (src[j+1] == '"' || src[j+1] == '\\'):
					j++
					w.quoted(src[j])
				default:
					w.quoted(d)
				}
			}
			if j >= len(src) {
				return line, errors.New("unterminated double quote")
			}
			i = j
		case c == '\\':
			if next == 0 {
				return line, errors.New("the line ends with a backslash")
			}
			if next == '\n' || controlByte(next) {
				return line, errors.New("a newline ends the line here; run one line and join commands with ; or &&")
			}
			start().quoted(next)
			i++
		default:
			if word == nil && c == '#' {
				return line, errors.New("comments (#) are not allowed")
			}
			if word == nil && c == '~' {
				return line, errors.New("~ is outside the worker's root; use a path inside the root")
			}
			start().unquoted(c)
		}
	}
	endWord()
	switch {
	case len(command) > 0:
		pipeline = append(pipeline, command)
		line.pipelines = append(line.pipelines, pipeline)
	case len(pipeline) > 0:
		return line, errors.New("the line ends with operator |")
	case len(line.ops) > 0 && line.ops[len(line.ops)-1] != ";":
		return line, fmt.Errorf("the line ends with operator %s", line.ops[len(line.ops)-1])
	case len(line.ops) > 0:
		line.ops = line.ops[:len(line.ops)-1] // a trailing ;
	}
	if len(line.pipelines) == 0 {
		return line, errors.New("empty command")
	}
	return line, nil
}

// maxGlobMatches bounds one glob's expansion.
const maxGlobMatches = 1000

// expandWord expands an unquoted glob against cwd as a shell would: matches
// are sorted, hidden names match only a pattern that starts with a dot, and
// a pattern that matches nothing stays literal. Every resulting path is
// confined later like any other argument.
func expandWord(root, cwd string, w shellWord) ([]string, error) {
	if !w.glob {
		return []string{w.value}, nil
	}
	full := w.pattern
	relative := !filepath.IsAbs(full)
	if relative {
		full = filepath.Join(cwd, full)
	}
	matches, err := filepath.Glob(full)
	if err != nil || len(matches) == 0 {
		return []string{w.value}, nil
	}
	if len(matches) > maxGlobMatches {
		return nil, fmt.Errorf("glob %q matches more than %d paths; narrow it", w.value, maxGlobMatches)
	}
	hidden := strings.HasPrefix(filepath.Base(w.pattern), ".")
	var out []string
	for _, match := range matches {
		if !hidden && strings.HasPrefix(filepath.Base(match), ".") {
			continue
		}
		if relative {
			if rel, err := filepath.Rel(cwd, match); err == nil {
				match = rel
			}
		}
		out = append(out, match)
	}
	if len(out) == 0 {
		return []string{w.value}, nil
	}
	return out, nil
}
