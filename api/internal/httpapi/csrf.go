package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

// A cookie-authenticated write must come from one of our own pages. A
// browser says where a request came from in Origin (always sent on unsafe
// requests), Referer, and Sec-Fetch-Site; a request from a foreign site is
// refused before any handler runs. Browsers never add an API key or a device
// token on their own, so requests carrying one are not checked. A request
// naming no source at all (a curl call) is allowed: a browser never omits
// all three on a cross-site request.

var errCSRF = apiErr(http.StatusForbidden, "csrf_rejected", "This request came from another site. Use the dashboard, or pass an API key.")

func unsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// originAllowed decides for one request from the three headers a browser
// sends; an empty string means the header is absent.
func originAllowed(allowed map[string]bool, origin, referer, secFetchSite string) bool {
	if strings.EqualFold(strings.TrimSpace(secFetchSite), "cross-site") {
		return false
	}
	if origin = strings.TrimSpace(origin); origin != "" {
		// "null" (a sandboxed or opaque origin) is not one of ours either.
		return allowed[strings.ToLower(strings.TrimRight(origin, "/"))]
	}
	if referer = strings.TrimSpace(referer); referer != "" {
		u, err := url.Parse(referer)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		return allowed[strings.ToLower(u.Scheme+"://"+u.Host)]
	}
	return true
}
