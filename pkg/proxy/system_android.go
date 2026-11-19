//go:build android

package proxy

import (
	"context"
	"net/url"
	"time"

	"github.com/ggdream/microlight/pkg/proxy/internal/androidproxy"
)

func systemRoutes(ctx context.Context, target *url.URL) ([]*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resolver, err := androidproxy.NewResolver()
	if err != nil {
		return nil, err
	}
	values, err := resolver.Routes(ctx, target.String())
	if err != nil {
		return nil, err
	}
	return parseRoutes(values)
}
