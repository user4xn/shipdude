package util

import (
	"net/http"
	"strconv"
	"strings"
)

// ParseInt64 parses a string into an int64.
func ParseInt64(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

// RequestScheme returns "https" when the request arrived over TLS or through a
// TLS-terminating proxy, otherwise "http".
func RequestScheme(r *http.Request) string {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return "https"
	}
	return "http"
}

// ServerURL reconstructs the externally visible base URL of the portal.
func ServerURL(r *http.Request) string {
	return RequestScheme(r) + "://" + r.Host
}

// IsLocalhostURL reports whether the URL points at a loopback host.
func IsLocalhostURL(url string) bool {
	return strings.Contains(url, "localhost") || strings.Contains(url, "127.0.0.1")
}

// FirstNonEmpty returns the first non-empty string from values.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
