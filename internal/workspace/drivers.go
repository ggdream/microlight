package workspace

import (
	"context"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
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

// CommandSession identifies the command journal across connection and process
// restarts. Replacing the journal must change this identity.
type CommandSession interface {
	CommandSessionID() string
}

type BrowserProvider interface {
	Browser() BrowserDriver
}

type FilesProvider interface {
	Files() ResolvedFileSystem
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
	Shell        filesystem.Shell
	Commands     backgroundshell.RecoverableShell
	Browser      BrowserDriver
	Computer     ComputerDriver
}

// DiscoverCommands checks the command capability independently of the
// interactive drivers. Recovery can run without a usable desktop session.
func DiscoverCommands(ctx context.Context, runtime Runtime) (backgroundshell.RecoverableShell, error) {
	declared, err := declaredCapabilities(ctx, runtime)
	if err != nil {
		return nil, err
	}
	return discoverCommands(runtime, declared), ctx.Err()
}

func declaredCapabilities(ctx context.Context, runtime Runtime) (Capabilities, error) {
	if provider, ok := runtime.(CapabilityProvider); ok {
		return provider.GetCapabilities(ctx)
	}
	return Capabilities{
		Files: true, Shell: true, Browser: true, RecoverableShell: true,
		Computer: ComputerCapabilities{Screenshot: true, ListWindows: true, Click: true, TypeText: true, PressKey: true},
	}, ctx.Err()
}

func discoverCommands(runtime Runtime, declared Capabilities) backgroundshell.RecoverableShell {
	if provider, ok := runtime.(CommandProvider); ok && declared.Shell && declared.RecoverableShell {
		return provider.Commands()
	}
	return nil
}

// DiscoverOperations probes the connected executor. Browser and desktop are
// independent, since a browser may be available in a headless environment.
func DiscoverOperations(ctx context.Context, runtime Runtime) (Operations, error) {
	declared, err := declaredCapabilities(ctx, runtime)
	if err != nil {
		return Operations{}, err
	}
	var ops Operations
	if declared.Files {
		ops.Files, _ = runtime.(ResolvedFileSystem)
		if provider, ok := runtime.(FilesProvider); ok {
			ops.Files = provider.Files()
		}
	}
	if declared.Shell {
		ops.Shell, _ = runtime.(filesystem.Shell)
	}
	ops.Commands = discoverCommands(runtime, declared)
	capabilities := &ops.Capabilities
	capabilities.Files = ops.Files != nil
	capabilities.Shell = ops.Shell != nil || ops.Commands != nil
	capabilities.RecoverableShell = ops.Commands != nil
	if provider, ok := runtime.(BrowserProvider); ok && declared.Browser {
		ops.Browser = provider.Browser()
		capabilities.Browser = ops.Browser != nil
	}
	if provider, ok := runtime.(ComputerProvider); ok && declared.Computer != (ComputerCapabilities{}) {
		ops.Computer = provider.Computer()
		if ops.Computer != nil {
			actual, err := ops.Computer.GetCapabilities(ctx)
			if err != nil {
				if ctx.Err() != nil || !(capabilities.Files || capabilities.Shell || capabilities.Browser) {
					return Operations{}, err
				}
				ops.Computer = nil
				return ops, nil
			}
			capabilities.Computer = ComputerCapabilities{
				Screenshot:  actual.Screenshot && declared.Computer.Screenshot,
				ListWindows: actual.ListWindows && declared.Computer.ListWindows,
				Click:       actual.Click && declared.Computer.Click,
				TypeText:    actual.TypeText && declared.Computer.TypeText,
				PressKey:    actual.PressKey && declared.Computer.PressKey,
			}
			if capabilities.Computer == (ComputerCapabilities{}) {
				ops.Computer = nil
			}
		}
	}
	return ops, ctx.Err()
}
