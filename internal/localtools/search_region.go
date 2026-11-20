package localtools

import (
	"sync/atomic"

	search "github.com/ggdream/microlight/pkg/websearch"
)

var searchRegion atomic.Value

// SetSearchRegion receives the host's regional formats from the app, without
// making network requests or depending on the selected UI language.
func SetSearchRegion(locale string) { searchRegion.Store(locale) }

func defaultSearchProvider() search.ID {
	locale, _ := searchRegion.Load().(string)
	return search.DefaultForRegion(locale)
}
