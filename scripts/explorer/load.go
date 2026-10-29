package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const formatFix = "make the file match the data format in docs/architecture/explorer/README.md"

// dataFile is one decoded content or proposal file.
type dataFile struct {
	Path     string // slash path relative to the repository root
	Content  *Content
	Proposal *Proposal
}

// loadData reads the manifest, compares it with the data files on disk, and
// decodes every listed file plus the examples, reporting each problem.
func (c *checker) loadData() []dataFile {
	listed := c.readManifest()
	var files []dataFile
	for _, entry := range listed {
		if entry == "facts.js" {
			continue // compared byte for byte with a fresh extraction
		}
		if file, ok := c.readDataFile(dataDir+"/"+entry, false); ok {
			files = append(files, file)
		}
	}
	examples, err := listJS(c.root, examplesDir)
	if err != nil {
		c.add(examplesDir, "examples", fmt.Sprintf("cannot list the examples: %v", err), "make the directory readable")
	}
	for _, name := range examples {
		if file, ok := c.readDataFile(examplesDir+"/"+name, true); ok {
			files = append(files, file)
		}
	}
	return files
}

// readManifest returns the manifest's entries that exist on disk and reports
// entries that are malformed or missing and data files it does not list.
func (c *checker) readManifest() []string {
	data, err := os.ReadFile(c.path(manifestPath))
	if err != nil {
		c.add(manifestPath, "manifest", "the manifest cannot be read", "create it with every data file listed")
		return nil
	}
	body, err := unwrap(manifestOpen, data)
	var entries []string
	if err == nil {
		err = decodeStrict(body, &entries)
	}
	if err != nil {
		c.add(manifestPath, "manifest", "malformed data: "+err.Error(), formatFix)
		return nil
	}
	onDisk, err := listJS(c.root, dataDir)
	if err != nil {
		c.add(dataDir, "data", fmt.Sprintf("cannot list the data files: %v", err), "make the directory readable")
	}
	exists := map[string]bool{}
	for _, name := range onDisk {
		exists[name] = true
	}
	seen := map[string]bool{}
	var listed []string
	for _, entry := range entries {
		item := "manifest entry " + entry
		switch {
		case !validDataPath(entry):
			c.add(manifestPath, item, "malformed data: entries are .js paths inside the data directory, other than manifest.js", formatFix)
		case seen[entry]:
			c.add(manifestPath, item, "malformed data: the entry is listed twice", "list each data file once")
		case !exists[entry]:
			c.add(manifestPath, item, "the listed data file does not exist", "create "+dataDir+"/"+entry+", or remove the entry")
		default:
			listed = append(listed, entry)
		}
		seen[entry] = true
	}
	for _, name := range onDisk {
		if !seen[name] {
			c.add(dataDir+"/"+name, "manifest", "the data file is not listed in the manifest",
				fmt.Sprintf("add %q to %s, or delete the file", name, manifestPath))
		}
	}
	return listed
}

func validDataPath(entry string) bool {
	return entry != "" && entry != "manifest.js" && strings.HasSuffix(entry, ".js") &&
		!strings.HasPrefix(entry, "/") && !strings.Contains(entry, `\`) &&
		path.Clean(entry) == entry && !strings.HasPrefix(entry, "../")
}

// listJS returns the .js files beneath dir, other than its manifest, as
// sorted slash paths relative to dir. A missing directory has none.
func listJS(root, dir string) ([]string, error) {
	base := filepath.Join(root, filepath.FromSlash(dir))
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, nil
	}
	var names []string
	err := filepath.WalkDir(base, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".js") {
			return nil
		}
		rel, err := filepath.Rel(base, file)
		if err != nil {
			return err
		}
		if rel = filepath.ToSlash(rel); rel != "manifest.js" {
			names = append(names, rel)
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}

// readDataFile decodes one content or proposal file. Examples must be
// proposals.
func (c *checker) readDataFile(rel string, example bool) (dataFile, bool) {
	malformed := func(msg string) (dataFile, bool) {
		c.add(rel, "file", "malformed data: "+msg, formatFix)
		return dataFile{}, false
	}
	data, err := os.ReadFile(c.path(rel))
	if err != nil {
		return malformed(err.Error())
	}
	body, err := unwrap(registerOpen, data)
	if err != nil {
		return malformed(err.Error())
	}
	var head header
	if err := json.Unmarshal(body, &head); err != nil {
		return malformed(err.Error())
	}
	if head.Format != formatVersion {
		c.add(rel, "format", fmt.Sprintf("format %d is not supported", head.Format), fmt.Sprintf("set format to %d", formatVersion))
		return dataFile{}, false
	}
	file := dataFile{Path: rel}
	switch {
	case head.Kind == "content" && !example:
		file.Content = &Content{}
		err = decodeStrict(body, file.Content)
	case head.Kind == "proposal":
		file.Proposal = &Proposal{}
		err = decodeStrict(body, file.Proposal)
	case example:
		return malformed(fmt.Sprintf("kind %q is not allowed here; examples are proposals", head.Kind))
	case head.Kind == "facts":
		return malformed("only the generated facts.js holds facts")
	default:
		return malformed(fmt.Sprintf("kind %q is not one of content, proposal", head.Kind))
	}
	if err != nil {
		return malformed(err.Error())
	}
	return file, true
}

// decodeStrict decodes strict JSON into v, refusing unknown and wrongly cased
// keys, which encoding/json alone would accept.
func decodeStrict(body []byte, v any) error {
	var generic any
	if err := json.Unmarshal(body, &generic); err != nil {
		return err
	}
	var problems []string
	strictKeys(reflect.TypeOf(v), generic, "", &problems)
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return json.Unmarshal(body, v)
}

// strictKeys walks a decoded JSON value beside the Go type it decodes into
// and records every object key that is not exactly a field's JSON name.
func strictKeys(t reflect.Type, v any, at string, problems *[]string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		object, ok := v.(map[string]any)
		if !ok {
			return // a type mismatch, reported by json.Unmarshal
		}
		fields := jsonFields(t)
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field, ok := fields[key]
			if !ok {
				*problems = append(*problems, fmt.Sprintf("%s has unknown field %q", describe(at, object), key))
				continue
			}
			strictKeys(field, object[key], join(at, key), problems)
		}
	case reflect.Slice:
		list, ok := v.([]any)
		if !ok {
			return
		}
		for i, item := range list {
			strictKeys(t.Elem(), item, fmt.Sprintf("%s[%d]", at, i), problems)
		}
	}
}

// jsonFields maps each JSON name of a struct, including promoted fields of
// embedded structs, to its type.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for name, typ := range jsonFields(f.Type) {
				fields[name] = typ
			}
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = f.Type
		}
	}
	return fields
}

func join(at, key string) string {
	if at == "" {
		return key
	}
	return at + "." + key
}

// describe names a JSON object for a message, with its id when it has one.
func describe(at string, object map[string]any) string {
	if at == "" {
		at = "the top-level object"
	}
	if id, ok := object["id"].(string); ok && id != "" {
		return fmt.Sprintf("%s (%s)", at, id)
	}
	return at
}
