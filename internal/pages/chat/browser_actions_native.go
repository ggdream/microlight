//go:build !js

package chat

type browserActions struct{}

func (*chatApp) deferBrowserAction(func()) bool { return false }
