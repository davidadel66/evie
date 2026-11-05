package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// namedFile is a file's bytes and the name the page's inputs know it by: a
// repository path as the map's meta gives it, or a design proposal's file
// name.
type namedFile struct {
	Name string
	Data []byte
}

// pageInputs are the files the page embeds beside the map, as check read
// them: the facts file and the changelog Evie's settings name, each nil
// when it is missing, and the design proposals, or why they could not be
// read.
type pageInputs struct {
	facts, changelog *namedFile
	proposals        []namedFile
	proposalsErr     error
}

// inputsHash is the hash the vision skill writes into the built page as
// vision-inputs-hash ("Inputs hash" in the skill's references/map-format.md),
// over the other files the page embeds: the facts file and the changelog the
// map's meta names (facts and changelog), then the design proposals in
// order. Each is one record: its kind and name on a line, its length in
// bytes on a line, its bytes with every CR LF as LF, and an LF. An input the
// map does not name, or whose file is missing, has no record: it is nil
// here. The result is the SHA-256 of the records joined, as lowercase hex
// after "sha256:".
func inputsHash(facts, changelog *namedFile, proposals []namedFile) string {
	h := sha256.New()
	record := func(kind string, f namedFile) {
		data := bytes.ReplaceAll(f.Data, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(h, "%s %s\n%d\n", kind, f.Name, len(data))
		h.Write(data)
		h.Write([]byte("\n"))
	}
	if facts != nil {
		record("facts", *facts)
	}
	if changelog != nil {
		record("changelog", *changelog)
	}
	for _, p := range proposals {
		record("proposal", p)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
