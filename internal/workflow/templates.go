package workflow

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Bundled templates (Stage 14 sub-stage 10, 2026-10-04): reviewed Workflow
// Definitions compiled into Evie. A template is only a source: proposing
// one to a Workspace commits exactly its reviewed files to procedural Git
// with its bundled dry-run fixtures, and the owner's normal approval
// activates it. Nothing here runs it or grants anything.
//
//go:embed templates
var bundled embed.FS

// templatesRoot is the bundled templates' folder; each subfolder is one
// definition named after it, with a fixtures.json for its dry run beside
// its reviewed files (never committed: only referenced files are).
const templatesRoot = "templates"

// TemplateInfo is one bundled template as a listing shows it.
type TemplateInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Inputs      []string `json:"inputs"`
}

// Templates lists the bundled templates by name.
func Templates() []TemplateInfo {
	entries, err := fs.ReadDir(bundled, templatesRoot)
	if err != nil {
		return nil
	}
	var out []TemplateInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(bundled, path.Join(templatesRoot, e.Name(), "workflow.yaml"))
		if err != nil {
			continue
		}
		def, problems := parseDefinition(data)
		if len(problems) > 0 || def.Name != e.Name() {
			continue
		}
		info := TemplateInfo{Name: def.Name, Description: strings.Join(strings.Fields(def.Description), " ")}
		for _, name := range sortedKeys(def.Inputs) {
			spec := def.Inputs[name]
			text := name + " (" + spec.Type
			if spec.Required {
				text += ", required"
			}
			info.Inputs = append(info.Inputs, text+")")
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b TemplateInfo) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// TemplateSource reads one bundled template's files.
func TemplateSource(name string) (Source, bool) {
	if !namePattern.MatchString(name) {
		return nil, false
	}
	sub, err := fs.Sub(bundled, path.Join(templatesRoot, name))
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "workflow.yaml"); err != nil {
		return nil, false
	}
	return fsSource{fsys: sub}, true
}

// TemplateFixtures is the dry-run fixture file bundled with a template.
func TemplateFixtures(name string) (Fixtures, error) {
	src, ok := TemplateSource(name)
	if !ok {
		return Fixtures{}, fmt.Errorf("no bundled workflow template named %q", name)
	}
	data, err := src.ReadFile("fixtures.json")
	if err != nil {
		return Fixtures{}, fmt.Errorf("template %s has no dry-run fixtures: %w", name, err)
	}
	return ParseFixtures(data)
}

// fsSource reads a definition from a read-only file system: only clean
// relative paths to regular files within the size limit.
type fsSource struct{ fsys fs.FS }

func (s fsSource) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("%s is not a path inside the definition", name)
	}
	info, err := fs.Stat(s.fsys, name)
	if err != nil {
		return nil, err
	}
	switch {
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("%s is not a regular file", name)
	case info.Size() > maxSourceFileBytes:
		return nil, fmt.Errorf("%s is larger than %d KiB", name, maxSourceFileBytes>>10)
	}
	return fs.ReadFile(s.fsys, name)
}
