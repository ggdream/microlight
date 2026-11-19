//go:build js

package proxy

import (
	"context"
	"net/url"
)

// The browser owns network routing; there is no application-visible proxy API.
func systemRoutes(context.Context, *url.URL) ([]*url.URL, error) { return nil, nil }
