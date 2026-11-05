package main

import (
	"bytes"
	"encoding/json"
)

// The facts file is a classic script: a fixed first line that opens a
// registration call, one strict JSON value, and a fixed last line that
// closes the call. The vision skill reads it in that wrapper.
const (
	factsPath      = "docs/architecture/explorer/data/facts.js"
	formatVersion  = 1
	extractCommand = "go run ./scripts/explorer extract"
)

// factsFile renders the facts as the facts file's text.
func factsFile(facts Facts) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("evieExplorer.register(\n")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(facts); err != nil {
		return nil, err
	}
	buf.WriteString(");\n")
	return buf.Bytes(), nil
}
