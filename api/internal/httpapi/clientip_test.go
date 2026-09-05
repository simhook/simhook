package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustsOnlyTheConfiguredHeader(t *testing.T) {
	var seen string
	h := clientIP("X-Real-IP")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.RemoteAddr }))

	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"proxy header", map[string]string{"X-Real-IP": "203.0.113.9"}, "203.0.113.9"},
		{"ipv6", map[string]string{"X-Real-IP": "2001:db8::1"}, "2001:db8::1"},
		{"client-suppliable headers are ignored", map[string]string{"X-Forwarded-For": "203.0.113.9", "True-Client-IP": "203.0.113.9"}, "192.0.2.1:1234"},
		{"garbage is ignored", map[string]string{"X-Real-IP": "not an address"}, "192.0.2.1:1234"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		for k, v := range tc.headers {
			req.Header.Set(k, v)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if seen != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, seen, tc.want)
		}
	}
}

func TestHostOf(t *testing.T) {
	if hostOf("192.0.2.1:1234") != "192.0.2.1" || hostOf("192.0.2.1") != "192.0.2.1" || hostOf("[2001:db8::1]:80") != "2001:db8::1" {
		t.Fatal("hostOf should strip a port and leave bare addresses alone")
	}
}
