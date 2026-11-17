package providerdiagnostic

import (
	"net/url"
	"regexp"
	"strings"
)

var link = regexp.MustCompile(`(?i)https?://[^\s<>"']+|\bBearer[ \t]+[A-Za-z0-9._~+/=%-]+`)

// Text keeps upstream diagnostics short and removes request credentials and URLs,
// which may contain signed download parameters.
func Text(message, credential string) string {
	if credential = strings.TrimSpace(credential); credential != "" {
		message = strings.NewReplacer(credential, "[redacted]", url.QueryEscape(credential), "[redacted]", url.PathEscape(credential), "[redacted]").Replace(message)
	}
	message = link.ReplaceAllString(message, "[redacted]")
	message = strings.Join(strings.Fields(message), " ")
	count := 0
	for offset := range message {
		if count == 400 {
			return message[:offset] + "…"
		}
		count++
	}
	return message
}
