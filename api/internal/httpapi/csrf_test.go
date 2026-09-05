package httpapi

import "testing"

func TestOriginAllowed(t *testing.T) {
	allowed := map[string]bool{"https://app.simhook.dev": true, "https://simhook.dev": true}
	cases := []struct {
		name                          string
		origin, referer, secFetchSite string
		want                          bool
	}{
		{"dashboard", "https://app.simhook.dev", "", "same-site", true},
		{"site", "https://simhook.dev", "https://simhook.dev/docs", "same-site", true},
		{"case and slash", "HTTPS://App.Simhook.dev/", "", "", true},
		{"foreign origin", "https://evil.example", "", "", false},
		{"foreign origin claiming same-site", "https://evil.example", "", "same-site", false},
		{"sibling host", "https://evil.simhook.dev", "", "same-site", false},
		{"opaque origin", "null", "", "", false},
		{"referer only, ours", "", "https://app.simhook.dev/devices?x=1", "", true},
		{"referer only, foreign", "", "https://evil.example/page", "", false},
		{"referer without scheme", "", "app.simhook.dev/devices", "", false},
		{"cross-site with no origin", "", "", "cross-site", false},
		{"cross-site with our origin", "https://app.simhook.dev", "", "cross-site", false},
		{"nothing at all", "", "", "", true},
		{"same-origin marker alone", "", "", "same-origin", true},
	}
	for _, tc := range cases {
		if got := originAllowed(allowed, tc.origin, tc.referer, tc.secFetchSite); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUnsafeMethod(t *testing.T) {
	for m, want := range map[string]bool{"GET": false, "HEAD": false, "OPTIONS": false, "POST": true, "PATCH": true, "PUT": true, "DELETE": true} {
		if got := unsafeMethod(m); got != want {
			t.Errorf("%s: %v", m, got)
		}
	}
}
