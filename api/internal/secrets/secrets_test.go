package secrets

import (
	"strings"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected format: %s", hash)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(hash, "correct horse battery stapl") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("garbage", "x") {
		t.Fatal("garbage hash accepted")
	}
}

func TestTokens(t *testing.T) {
	tok, hash, err := NewToken(PrefixAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "sh_") || len(tok) < 40 {
		t.Fatalf("bad token %q", tok)
	}
	if string(hash) != string(Hash(tok)) {
		t.Fatal("hash mismatch")
	}
	if DisplayPrefix(tok) != tok[:12] {
		t.Fatal("prefix")
	}
	code, chash, err := NewPairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("bad pairing code %q", code)
	}
	if string(chash) != string(Hash(NormalizePairingCode(strings.ToLower(" "+code+" ")))) {
		t.Fatal("pairing code normalization must be case and whitespace insensitive")
	}
	otp, _, err := NewOneTimeCode()
	if err != nil || len(otp) != 6 {
		t.Fatalf("bad one-time code %q %v", otp, err)
	}
}

func TestBox(t *testing.T) {
	box, err := NewBox([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := box.Seal([]byte("whsec_secret"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := box.Open(ct)
	if err != nil || string(pt) != "whsec_secret" {
		t.Fatalf("round trip failed: %q %v", pt, err)
	}
	ct[len(ct)-1] ^= 1
	if _, err := box.Open(ct); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestWebhookSignature(t *testing.T) {
	body := []byte(`{"id":"1","event":"ping"}`)
	sig := SignWebhook("whsec_abc", 1700000000, body)
	if !strings.HasPrefix(sig, "t=1700000000,v1=") {
		t.Fatalf("bad header %q", sig)
	}
	if !VerifyWebhook("whsec_abc", sig, body, 1700000100, 300) {
		t.Fatal("valid signature rejected")
	}
	if VerifyWebhook("whsec_abc", sig, body, 1700001000, 300) {
		t.Fatal("stale signature accepted")
	}
	if VerifyWebhook("whsec_other", sig, body, 1700000100, 300) {
		t.Fatal("wrong secret accepted")
	}
	if VerifyWebhook("whsec_abc", sig, []byte(`{"id":"2"}`), 1700000100, 300) {
		t.Fatal("different body accepted")
	}
}
