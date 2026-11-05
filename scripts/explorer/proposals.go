package main

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// visionProposalsDir holds the design proposals of features in flight, one
// file per feature, beside the map ("Design proposals" in the skill's
// references/map-format.md: proposals/ beside a map named map.json). It
// exists only while a proposal does.
const visionProposalsDir = ".vision/proposals"

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
