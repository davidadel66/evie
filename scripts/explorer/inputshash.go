package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// inputsHash is the hash the vision skill writes into the built page as
// vision-inputs-hash ("Inputs hash" in the skill's references/map-format.md),
// over the other files the page embeds: the facts file and the changelog the
// map's meta names (facts and changelog), then the design proposals in
// order. Each is one record: its kind and name on a line, its length in
// bytes on a line, its bytes with every CR LF as LF, and an LF. An input the
// map does not name, or whose file is missing, has no record. The result is
// the SHA-256 of the records joined, as lowercase hex after "sha256:".
func inputsHash(root, facts, changelog string, proposals []proposalFile) (string, error) {
	h := sha256.New()
	record := func(kind, name string, data []byte) {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(h, "%s %s\n%d\n", kind, name, len(data))
		h.Write(data)
		h.Write([]byte("\n"))
	}
	for _, input := range []struct{ kind, name string }{{"facts", facts}, {"changelog", changelog}} {
		if input.name == "" {
			continue
		}
		data, ok, err := readRegular(filepath.Join(root, filepath.FromSlash(input.name)))
		if err != nil {
			return "", err
		}
		if ok {
			record(input.kind, input.name, data)
		}
	}
	for _, p := range proposals {
		record("proposal", p.Name, p.Data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
