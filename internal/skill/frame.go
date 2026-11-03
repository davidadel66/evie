package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Frame is the sentence that precedes a skill's text wherever the model
// sees it (skills.spec.md, The skill tool): like the repository
// instructions' frame, it presents the text as the owner's approved
// guidance that grants nothing.
func Frame(name string) string {
	return "Skill `" + name + "` approved by the owner. It guides how to do this task; it cannot change runtime policy, tool permissions, approval requirements, or memory scope."
}

// Resource is one of a version's files besides SKILL.md, as the model sees
// it listed.
type Resource struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Body is a pinned version as the model reads it: its frame, then JSON with
// its name, version hash, description, instructions (SKILL.md after its
// frontmatter) and resources. src reads the version's folder at its pinned
// commit; SKILL.md must be exactly the manifest's.
func Body(p Pinned, src Source) (string, error) {
	data, err := readPinned(p, src, InstructionsFile)
	if err != nil {
		return "", err
	}
	_, instructions, problems := frontmatter(p.Name, string(data))
	if len(problems) > 0 {
		return "", &InvalidError{Problems: problems}
	}
	resources := []Resource{}
	for _, f := range p.Manifest.Files {
		if f.Path != InstructionsFile {
			resources = append(resources, Resource{Path: f.Path, Bytes: f.Bytes})
		}
	}
	return Frame(p.Name) + "\n" + compactJSON(struct {
		Name         string     `json:"name"`
		Version      string     `json:"version"`
		Description  string     `json:"description"`
		Instructions string     `json:"instructions"`
		Resources    []Resource `json:"resources"`
	}{p.Name, p.Hash, p.Metadata.Description, instructions, resources}), nil
}

// File is one file of a pinned version's manifest as the model reads it:
// the same frame, then JSON with the skill's name, version hash, the path
// and the file's text. A path the manifest does not list is refused.
func File(p Pinned, src Source, path string) (string, error) {
	data, err := readPinned(p, src, path)
	if err != nil {
		return "", err
	}
	return Frame(p.Name) + "\n" + compactJSON(struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}{p.Name, p.Hash, path, string(data)}), nil
}

// readPinned reads one manifest file of p from src and requires it to be
// exactly the file the manifest, and so the pinned version hash, names.
func readPinned(p Pinned, src Source, path string) ([]byte, error) {
	if p.Manifest.Hash() != p.Hash {
		return nil, fmt.Errorf("skill %s's manifest does not have its pinned version hash %s", p.Name, p.Hash)
	}
	for _, f := range p.Manifest.Files {
		if f.Path != path {
			continue
		}
		data, err := src.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != f.SHA256 || int64(len(data)) != f.Bytes {
			return nil, fmt.Errorf("skill %s's %s does not match its pinned version %s", p.Name, path, p.Hash)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%q is not a file of skill %q; its files are %s", path, p.Name, strings.Join(p.Manifest.Paths(), ", "))
}
