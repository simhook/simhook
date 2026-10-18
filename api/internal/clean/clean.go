// Package clean repairs strings a phone reports before they reach the
// database. Postgres refuses a NUL byte in text and jsonb alike, and some
// handsets decode a character of an SMS as U+0000, so without this one such
// text would fail to store forever while the phone kept retrying it, with
// every later report waiting in line behind it.
package clean

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Text drops NUL characters and replaces invalid UTF-8 with U+FFFD. Any
// other text comes back as it was.
func Text(s string) string {
	if utf8.ValidString(s) && !strings.ContainsRune(s, 0) {
		return s
	}
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "�")
}

// TextPtr is Text for an optional string. nil stays nil.
func TextPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := Text(*p)
	return &s
}

// JSON applies Text to every string in a JSON document, keys included, and
// returns the document re-encoded with its numbers untouched. A document
// with nothing to repair comes back as it was, byte for byte, and so does
// anything that is not JSON, for the validation layer to refuse.
func JSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || (utf8.Valid(raw) && !bytes.ContainsRune(raw, 0) && !bytes.Contains(raw, []byte(`\u0000`))) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw
	}
	out, err := json.Marshal(walk(v))
	if err != nil {
		return raw
	}
	return out
}

func walk(v any) any {
	switch t := v.(type) {
	case string:
		return Text(t)
	case []any:
		for i := range t {
			t[i] = walk(t[i])
		}
		return t
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[Text(k)] = walk(val)
		}
		return m
	default:
		return v
	}
}
