// Package androidproxy resolves Android's current system proxy through the JVM.
package androidproxy

import "context"

// Resolver reads routes from the platform's proxy selector on each request.
//
//mygo:native
type Resolver interface {
	// Routes returns ordered proxy URLs, or direct:// for an explicit direct
	// route. Wi-Fi exclusions and PAC are evaluated by Android itself.
	Routes(ctx context.Context, target string) ([]string, error)
}
