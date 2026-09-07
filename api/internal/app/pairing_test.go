package app_test

import (
	"strings"
	"testing"
)

// A person typing a code from the dashboard gets told what is wrong with it:
// the code itself, or that it is spent. The shape it is typed in is not one
// of the things that can be wrong.
func TestAPairingCodeSaysWhyItWasRefused(t *testing.T) {
	h := startApp(t)
	account := h.signUp(t, "pairing@example.com")

	r := account.must("POST", "/v1/devices/pairing-codes", nil, 201)
	shown := str(r.body, "code")
	if len(shown) != 9 || shown[4] != '-' {
		t.Fatalf("the dashboard shows a dashed code, got %q", shown)
	}
	phone := &client{t: t, base: h.srv.URL}
	pair := func(code, hardwareKey string) resp {
		return phone.do("POST", "/v1/device/pair", map[string]any{"code": code, "hardware_key": hardwareKey})
	}

	// Typed straight through, in lower case: the same code.
	if r := pair(strings.ToLower(strings.ReplaceAll(shown, "-", "")), "hw-typed"); r.status != 201 {
		t.Fatalf("undashed lower-case code: %d %s", r.status, r.raw)
	}

	// The same code a second time is spent, not wrong.
	r = pair(shown, "hw-second")
	if r.status != 400 || str(r.body, "code") != "pairing_code_used" {
		t.Fatalf("spent code: %d %s", r.status, r.raw)
	}

	// A code past its ten minutes says so.
	r = account.must("POST", "/v1/devices/pairing-codes", nil, 201)
	execSQL(t, `update pairing_codes set expires_at = now() - interval '1 minute' where id = $1`, str(r.body, "id"))
	r = pair(str(r.body, "code"), "hw-late-phone")
	if r.status != 400 || str(r.body, "code") != "pairing_code_expired" {
		t.Fatalf("expired code: %d %s", r.status, r.raw)
	}
	if !strings.Contains(str(r.body, "message"), "make a new one") {
		t.Fatalf("the expired message should say what to do, got %q", str(r.body, "message"))
	}

	// A code nobody was ever shown is wrong.
	r = pair("ZZZZ-ZZZZ", "hw-guess")
	if r.status != 400 || str(r.body, "code") != "invalid_pairing_code" {
		t.Fatalf("unknown code: %d %s", r.status, r.raw)
	}
}
