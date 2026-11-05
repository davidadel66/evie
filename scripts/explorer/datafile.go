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
	registerOpen   = "evieExplorer.register("
	wrapperClose   = ");"
	formatVersion  = 1
	extractCommand = "go run ./scripts/explorer extract"
)

// wrap renders value as a file body inside the given registration call.
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

// hasCRLF reports whether a file has CRLF line endings.
func hasCRLF(data []byte) bool {
	return bytes.Contains(data, []byte("\r\n"))
}

// lineEndings reports a file with CRLF line endings.
func (c *checker) lineEndings(file string, data []byte) {
	if hasCRLF(data) {
		c.add(file, "file", "the file has CRLF line endings",
			"convert it to LF line endings; the repository's .gitattributes checks the explorer's files out with LF")
	}
}
