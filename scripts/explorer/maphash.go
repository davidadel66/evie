package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// mapHash is the system map's hash as the vision skill writes it into the
// built page ("Map hash" in the skill's references/map-format.md): the map
// parsed as JSON, without what stamp and publishing write, serialized with
// the JSON Canonicalization Scheme (RFC 8785), then SHA-256 as lowercase hex
// after "sha256:". It covers the map's content, not its layout.
func mapHash(data []byte) (string, error) {
	value, err := decodeJSONValue(data)
	if err != nil {
		return "", err
	}
	stripUnhashed(value)
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// decodeJSONValue decodes one JSON value, keeping numbers as their literal
// text so canonicalJSON formats them from the exact literal.
func decodeJSONValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("there is more than one JSON value")
	}
	return value, nil
}

// stripUnhashed removes what the hash leaves out: meta.commit, meta.stamped
// and meta.artifact, and from every code entry its stamp hash and, in an
// entry that reads the repository (symbol, lines or cell), the copy of the
// code that resolving it fills in.
func stripUnhashed(value any) {
	root, ok := value.(map[string]any)
	if !ok {
		return
	}
	if meta, ok := root["meta"].(map[string]any); ok {
		for _, key := range []string{"commit", "stamped", "artifact"} {
			delete(meta, key)
		}
	}
	code, ok := root["code"].(map[string]any)
	if !ok {
		return
	}
	for _, e := range code {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		delete(entry, "hash")
		if hasAnyKey(entry, "symbol", "lines", "cell") {
			for _, key := range []string{"src", "startLine", "where"} {
				delete(entry, key)
			}
		}
	}
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

// canonicalJSON serializes a decoded JSON value per RFC 8785: object keys
// sorted by their UTF-16 code units, no whitespace, numbers as ECMAScript
// writes them, and strings escaping only what JSON requires.
func canonicalJSON(value any) ([]byte, error) {
	var b bytes.Buffer
	if err := writeCanonical(&b, value); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeCanonical(b *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(v))
	case json.Number:
		f, err := strconv.ParseFloat(v.String(), 64)
		if err != nil || math.IsInf(f, 0) {
			return fmt.Errorf("the number %s is outside what an IEEE 754 double holds", v)
		}
		b.WriteString(esNumber(f))
	case string:
		writeCanonicalString(b, v)
	case []any:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		// RFC 8785 sorts keys by their UTF-16 code units: a character
		// outside the Basic Multilingual Plane sorts by its surrogates,
		// before U+E000 to U+FFFF. Each key is encoded once.
		keys := make([]utf16Key, 0, len(v))
		for key := range v {
			keys = append(keys, utf16Key{key, utf16.Encode([]rune(key))})
		}
		slices.SortFunc(keys, func(a, b utf16Key) int { return slices.Compare(a.units, b.units) })
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeCanonicalString(b, key.text)
			b.WriteByte(':')
			if err := writeCanonical(b, v[key.text]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("a %T is not a JSON value", value)
	}
	return nil
}

// utf16Key is an object key and its UTF-16 code units, which it sorts by.
type utf16Key struct {
	text  string
	units []uint16
}

// writeCanonicalString writes s as a JSON string escaping only the
// quotation mark, the reverse solidus and the control characters below
// U+0020, which use their short forms or lowercase \u00xx.
func writeCanonicalString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

// esNumber writes a finite double as ECMAScript's Number.prototype.toString
// does (ECMA-262 Number::toString, which RFC 8785 section 3.2.2.3 adopts):
// the shortest digits that round-trip, in plain notation for exponents up to
// 21 and down to -6, and in exponent notation otherwise.
func esNumber(f float64) string {
	if f == 0 {
		return "0" // including -0
	}
	if f < 0 {
		return "-" + esNumber(-f)
	}
	// 'e' with precision -1 gives the shortest round-trip digits as
	// d.ddde±xx: the value is digits × 10^(n-k) with k digits.
	mantissa, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	e, _ := strconv.Atoi(exp) // FormatFloat always writes a valid exponent
	k, n := len(digits), e+1
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}
	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	exponent := "e" + sign + strconv.Itoa(abs(n-1))
	if k == 1 {
		return digits + exponent
	}
	return digits[:1] + "." + digits[1:] + exponent
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
