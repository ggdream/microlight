package workspace

import (
	"context"

	"github.com/cloudwego/eino/adk/filesystem"
)

type Image struct {
	MIMEType string
	Data     []byte
	X, Y     int
}

type WindowInfo struct {
	ID    string
	Title string
}

type ComputerCapabilities struct {
	Screenshot, ListWindows, Click, TypeText, PressKey bool
}

// ComputerDriver targets the interactive desktop of the selected environment.
type ComputerDriver interface {
	GetCapabilities(context.Context) (ComputerCapabilities, error)
	Screenshot(context.Context) (Image, error)
	ListWindows(context.Context) ([]WindowInfo, error)
	Click(ctx context.Context, x, y int) error
	TypeText(ctx context.Context, text string) error
	PressKey(ctx context.Context, key string) error
}

type TabInfo struct {
	ID, URL, Title string
}

type BrowserSnapshot struct {
	URL, Title, Text string
	Controls         []BrowserControl
}

type BrowserControl struct {
	Label, Selector string
}

// BrowserDriver uses explicit tab IDs so calls do not depend on shared focus.
type BrowserDriver interface {
	ListTabs(context.Context) ([]TabInfo, error)
	Navigate(ctx context.Context, tabID, url string) error
	Snapshot(ctx context.Context, tabID string) (BrowserSnapshot, error)
	Click(ctx context.Context, tabID, selector string) error
}

type ComputerProvider interface {
	Computer() ComputerDriver
}

// ComputerSession identifies an interactive desktop across reconnects.
type ComputerSession interface {
	ComputerSessionID() string
}

type BrowserProvider interface {
	Browser() BrowserDriver
}

// BrowserSession identifies the same browser state across executor reconnects.
// Replace the identity when launching a different browser or profile.
type BrowserSession interface {
	BrowserSessionID() string
}

type Capabilities struct {
	Files, Shell, Browser bool
	RecoverableShell      bool
	Computer              ComputerCapabilities
}

// CapabilityProvider can limit the operations offered by a partially
// available environment even when it implements the same Go interfaces.
type CapabilityProvider interface {
	GetCapabilities(context.Context) (Capabilities, error)
}

type Operations struct {
	Capabilities Capabilities
	Files        ResolvedFileSystem
	Browser      BrowserDriver
	Computer     ComputerDriver
}

// DiscoverOperations probes the connected executor. Browser and desktop are
// independent, since a browser may be available in a headless environment.
func DiscoverOperations(ctx context.Context, runtime Runtime) (Operations, error) {
	var ops Operations
	ops.Files, _ = runtime.(ResolvedFileSystem)
	_, shell := runtime.(filesystem.Shell)
	capabilities := &ops.Capabilities
	capabilities.Files, capabilities.Shell = ops.Files != nil, shell
	if provider, ok := runtime.(CapabilityProvider); ok {
		discovered, err := provider.GetCapabilities(ctx)
		if err != nil {
			return Operations{}, err
		}
		capabilities.Files = capabilities.Files && discovered.Files
		capabilities.Shell = capabilities.Shell && discovered.Shell
		if !capabilities.Files {
			ops.Files = nil
		}
	}
	if provider, ok := runtime.(BrowserProvider); ok {
		ops.Browser = provider.Browser()
		capabilities.Browser = ops.Browser != nil
	}
	if provider, ok := runtime.(CommandProvider); ok && capabilities.Shell {
		capabilities.RecoverableShell = provider.Commands() != nil
	}
	if provider, ok := runtime.(ComputerProvider); ok {
		ops.Computer = provider.Computer()
		if ops.Computer != nil {
			var err error
			capabilities.Computer, err = ops.Computer.GetCapabilities(ctx)
			if err != nil {
				return Operations{}, err
			}
		}
	}
	return ops, ctx.Err()
}
