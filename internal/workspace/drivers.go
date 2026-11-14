package workspace

import "context"

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

// DiscoverCapabilities probes the connected executor. Browser and desktop are
// independent, since a browser may be available in a headless environment.
func DiscoverCapabilities(ctx context.Context, runtime Runtime) (Capabilities, error) {
	capabilities := Capabilities{Files: true, Shell: true}
	if provider, ok := runtime.(CapabilityProvider); ok {
		discovered, err := provider.GetCapabilities(ctx)
		if err != nil {
			return Capabilities{}, err
		}
		capabilities.Files, capabilities.Shell = discovered.Files, discovered.Shell
	}
	if provider, ok := runtime.(BrowserProvider); ok {
		capabilities.Browser = provider.Browser() != nil
	}
	if provider, ok := runtime.(CommandProvider); ok && capabilities.Shell {
		capabilities.RecoverableShell = provider.Commands() != nil
	}
	if provider, ok := runtime.(ComputerProvider); ok {
		if driver := provider.Computer(); driver != nil {
			var err error
			capabilities.Computer, err = driver.GetCapabilities(ctx)
			if err != nil {
				return capabilities, err
			}
		}
	}
	return capabilities, ctx.Err()
}
