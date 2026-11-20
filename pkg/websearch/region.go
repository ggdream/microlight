package websearch

import (
	"strings"

	"golang.org/x/text/language"
)

// DefaultForRegion uses the explicit system region, independently of the app's
// display language. A missing/unknown region falls back to DuckDuckGo.
func DefaultForRegion(locale string) ID {
	locale = strings.Split(strings.Split(locale, ".")[0], "@")[0]
	tag, err := language.Parse(strings.ReplaceAll(locale, "_", "-"))
	if err == nil {
		_, _, region := tag.Raw()
		if region.String() == "CN" {
			return BingCN
		}
	}
	return DuckDuckGo
}
