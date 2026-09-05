package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// The route has already applied its byte ceiling. Review envelopes have a
// different ceiling from compiler model responses, but retain strict UTF-8,
// unique object keys, bounded nesting and exactly one JSON value.
func validateCandidateJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return errors.New("invalid review JSON encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := candidateJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing review JSON")
	}
	return nil
}
func candidateJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("review JSON nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate review JSON key")
			}
			seen[name] = true
			if err := candidateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := candidateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid review JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}
