package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// visionProposalsDir holds the design proposals of features in flight, one
// file per feature, beside the map ("Design proposals" in the skill's
// references/map-format.md: proposals/ beside a map named map.json). It
// exists only while a proposal does.
const visionProposalsDir = ".vision/proposals"

// proposalStatuses are the statuses the vision skill knows: proposed, then
// accepted once the design is agreed, building once work starts; example
// marks a proposal that only shows the format.
var proposalStatuses = []string{"proposed", "accepted", "building", "example"}

// proposalFile is one design proposal file as stored.
type proposalFile struct {
	Name string // the file name, such as top-k.json
	Data []byte
}

// readProposals reads the design proposals as the vision skill finds them:
// every regular .json file directly in the folder whose name does not start
// with a dot, sorted by file name as UTF-8 bytes. Without the folder there
// are none.
func readProposals(root string) ([]proposalFile, error) {
	dir := filepath.Join(root, filepath.FromSlash(visionProposalsDir))
	if info, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir) // sorted by name, byte by byte
	if err != nil {
		return nil, err
	}
	var files []proposalFile
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || path.Ext(name) != ".json" {
			continue
		}
		data, ok, err := readRegular(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if ok {
			files = append(files, proposalFile{Name: name, Data: data})
		}
	}
	return files, nil
}

// readRegular reads a regular file, following a symbolic link to one. It
// reports false, and no error, when there is no such file: nothing there, or
// a folder or another kind of file.
func readRegular(file string) ([]byte, bool, error) {
	info, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.Mode().IsRegular() {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(file)
	return data, err == nil, err
}

// validateProposal holds a design proposal to the minimum CI can check
// without the vision skill: the file is one JSON object, its id is its file
// name, and its status is one the skill knows. Everything else (its fields,
// statements, references and operations on the map) is the skill's own
// check, which its build runs first.
func (c *checker) validateProposal(p proposalFile) {
	file := visionProposalsDir + "/" + p.Name
	then := ", then " + visionRebuildInputs
	format := visionFormat + ` ("Design proposals")`
	value, err := decodeJSONValue(p.Data)
	if err != nil {
		c.add(file, "proposal", "the file is not valid JSON: "+err.Error(), "write it as one JSON object in the format of "+format+then)
		return
	}
	proposal, ok := value.(map[string]any)
	if !ok {
		c.add(file, "proposal", "the file is not a JSON object", "write it as one JSON object in the format of "+format+then)
		return
	}
	name := strings.TrimSuffix(p.Name, ".json")
	switch id, ok := proposal["id"]; {
	case !ok:
		c.add(file, "id", "the proposal has no id", fmt.Sprintf("set id to %q, its file's name", name)+then)
	case id != name:
		fix := fmt.Sprintf("set id to %q", name)
		if s, ok := id.(string); ok {
			fix += fmt.Sprintf(", or rename the file %s.json", s)
		}
		c.add(file, "id", fmt.Sprintf("id %s does not match the file name", jsonText(id)), fix+then)
	}
	const statusFix = "set it to proposed, accepted once the design is agreed, or building once work starts, and delete the proposal once its feature merges"
	switch raw, ok := proposal["status"]; {
	case !ok:
		c.add(file, "status", "the proposal has no status", statusFix+then)
	case !slices.Contains(proposalStatuses, stringOr(raw)):
		c.add(file, "status", fmt.Sprintf("status %s is not one of proposed, accepted, building or example", jsonText(raw)), statusFix+then)
	}
}

// stringOr returns a decoded JSON string, or "" for any other value.
func stringOr(v any) string {
	s, _ := v.(string)
	return s
}

// jsonText writes a decoded JSON value as JSON, for a problem's message.
func jsonText(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}
