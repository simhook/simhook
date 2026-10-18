package clean

import (
	"encoding/json"
	"testing"
)

func TestText(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"Kodunuz 4821. İyi günler": "Kodunuz 4821. İyi günler",
		"çağrı \U0001F600":         "çağrı \U0001F600",
		"Kod\x00unuz":              "Kodunuz",
		"\x00\x00":                 "",
		"abc\xffdef":               "abc�def",
		"a\x00b\xfe":               "ab�",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTextPtr(t *testing.T) {
	if TextPtr(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	s := "x\x00y"
	if got := TextPtr(&s); got == nil || *got != "xy" {
		t.Fatalf("TextPtr = %v", got)
	}
	if s != "x\x00y" {
		t.Fatal("the caller's string must not change")
	}
}

func TestJSON(t *testing.T) {
	in := json.RawMessage(`[{"carrier":"Turk\u0000cell","subscription_id":1867719143000,"slot":0,"tags":["a\u0000","b"],"na\u0000me":null}]`)
	got := JSON(in)
	var v []map[string]any
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("output is not JSON: %v: %s", err, got)
	}
	if v[0]["carrier"] != "Turkcell" {
		t.Errorf("carrier = %q", v[0]["carrier"])
	}
	if _, ok := v[0]["name"]; !ok {
		t.Errorf("key not cleaned: %s", got)
	}
	if tags := v[0]["tags"].([]any); tags[0] != "a" {
		t.Errorf("nested string not cleaned: %v", tags)
	}
	if string(got) == string(in) {
		t.Error("document with NUL came back unchanged")
	}
	// Numbers survive exactly, not as floats.
	if want := `"subscription_id":1867719143000`; !contains(got, want) {
		t.Errorf("number changed: %s", got)
	}

	clean := json.RawMessage(`{"battery_percent":99,"timezone":"Europe/Istanbul","note":"\\u0000 is a literal backslash here"}`)
	if got := JSON(clean); string(got) != string(clean) {
		// The escaped backslash is not a NUL; the document must only be
		// re-encoded if it held one. Either way it must still say the same.
		var a, b any
		_ = json.Unmarshal(clean, &a)
		_ = json.Unmarshal(got, &b)
		if ja, _ := json.Marshal(a); string(ja) != string(mustMarshal(b)) {
			t.Errorf("clean document changed meaning: %s -> %s", clean, got)
		}
	}
	if got := JSON(nil); got != nil {
		t.Errorf("nil -> %v", got)
	}
	if bad := json.RawMessage(`{"x":"\u0000`); string(JSON(bad)) != string(bad) {
		t.Error("invalid JSON must come back as it was")
	}
}

func contains(b json.RawMessage, s string) bool {
	return len(b) > 0 && string(b) != "" && indexOf(string(b), s) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
