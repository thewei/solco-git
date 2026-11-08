package nativeui

import (
	"net/url"
	"strings"
)

// safeExternalURL reports a link the system may open for a rendered document:
// http, https or mailto, never file:, javascript:, data: or a custom scheme.
func safeExternalURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https":
		return parsed.Host != ""
	case "mailto":
		return parsed.Opaque != ""
	}
	return false
}
