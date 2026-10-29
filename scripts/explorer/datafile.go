package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Every explorer data file is a classic script: a fixed first line that opens
// the viewer's registration call, one strict JSON value, and a fixed last line
// that closes the call. The manifest uses its own registration call.
const (
	dataDir        = "docs/architecture/explorer/data"
	examplesDir    = "docs/architecture/explorer/examples"
	factsPath      = dataDir + "/facts.js"
	manifestPath   = dataDir + "/manifest.js"
	registerOpen   = "evieExplorer.register("
	manifestOpen   = "evieExplorer.manifest("
	wrapperClose   = ");"
	formatVersion  = 1
	extractCommand = "go run ./scripts/explorer extract"
)

// wrap renders value as a data file body inside the given registration call.
func wrap(open string, value any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(open + "\n")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	buf.WriteString(wrapperClose + "\n")
	return buf.Bytes(), nil
}

// unwrap returns the JSON body of a data file after checking its fixed first
// and last lines.
func unwrap(open string, data []byte) ([]byte, error) {
	text := strings.TrimSuffix(string(data), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("want the line %q, a JSON body, and the line %q", open, wrapperClose)
	}
	if lines[0] != open {
		return nil, fmt.Errorf("first line is %q, want exactly %q", lines[0], open)
	}
	if lines[len(lines)-1] != wrapperClose {
		return nil, fmt.Errorf("last line is %q, want exactly %q", lines[len(lines)-1], wrapperClose)
	}
	return []byte(strings.Join(lines[1:len(lines)-1], "\n")), nil
}
