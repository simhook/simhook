package webhooks

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/simhook/simhook/internal/config"
)

func TestValidateURL(t *testing.T) {
	public := New(nil, &config.Config{WebhookTimeoutSeconds: 5}, nil, nil, nil)
	lan := New(nil, &config.Config{WebhookTimeoutSeconds: 5, WebhookAllowPrivateHosts: true}, nil, nil, nil)

	cases := []struct {
		url        string
		public, lan bool
	}{
		{"https://example.com/hooks", true, true},
		{"https://example.com:8443/hooks?x=1", true, true},
		{"http://example.com/hooks", false, true},
		{"https://user:pass@example.com/", false, false},
		{"ftp://example.com/", false, false},
		{"https://", false, false},
		{"not a url", false, false},
		{"https://localhost/hooks", false, true},
		{"https://printer.local/hooks", false, true},
		{"https://10.0.0.5/hooks", false, true},
		{"https://192.168.1.10:9000/", false, true},
		{"https://100.64.1.1/", false, true},
		{"https://[::1]/", false, true},
		{"https://169.254.169.254/latest", false, true},
		{"https://203.0.113.9/", true, true},
	}
	for _, tc := range cases {
		if got := public.ValidateURL(tc.url) == nil; got != tc.public {
			t.Errorf("public %q: accepted=%v, want %v", tc.url, got, tc.public)
		}
		if got := lan.ValidateURL(tc.url) == nil; got != tc.lan {
			t.Errorf("lan %q: accepted=%v, want %v", tc.url, got, tc.lan)
		}
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("a\x00b"); got != "ab" {
		t.Fatalf("NUL should be dropped, got %q", got)
	}
	if got := excerpt("caf\xc3"); !utf8.ValidString(got) || !strings.HasSuffix(got, "�") {
		t.Fatalf("a broken sequence should become U+FFFD, got %q", got)
	}
	long := strings.Repeat("ş", responseExcerptLimit+10)
	if got := excerpt(long); utf8.RuneCountInString(got) != responseExcerptLimit || !utf8.ValidString(got) {
		t.Fatalf("the cut must be on a character boundary: %d runes, valid=%v", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
}

func TestNormalizeName(t *testing.T) {
	if n, err := normalizeName(nil); n != nil || err != nil {
		t.Fatal("nil stays nil")
	}
	blank := "   "
	if n, err := normalizeName(&blank); n != nil || err != nil {
		t.Fatal("blank means unnamed")
	}
	ok := "  Production  "
	if n, err := normalizeName(&ok); err != nil || *n != "Production" {
		t.Fatalf("trim: %v %v", n, err)
	}
	long := strings.Repeat("ç", 65)
	if _, err := normalizeName(&long); err == nil {
		t.Fatal("65 characters should be refused")
	}
	edge := strings.Repeat("ç", 64)
	if _, err := normalizeName(&edge); err != nil {
		t.Fatal("64 characters of two bytes each should pass")
	}
}
