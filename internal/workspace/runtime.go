package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/ggdream/microlight/assets/locales"
)

type Target struct {
	Kind          string
	Directory     string
	DockerContext string
	Engine        ContainerEngine `json:",omitzero"`
	Container     string
	Managed       bool
	Host          string
	Port          string
	User          string
	KeyFile       string
	Auth          string
	HostKey       string
	Credential    string
	// Resource identifies an environment managed by the connector for Kind.
	Resource string `json:",omitempty"`
}

type Environment struct {
	OS, Arch, Shell string
}

type FileSystem interface {
	LsInfo(context.Context, *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error)
	Read(context.Context, *filesystem.ReadRequest) (*filesystem.FileContent, error)
	Write(context.Context, *filesystem.WriteRequest) error
}

type ResolvedFileSystem interface {
	FileSystem
	Resolve(context.Context, string, bool) (path, version string, err error)
}

// Runtime owns the connection to an execution environment. Its operations
// are discovered separately.
type Runtime interface {
	Environment() Environment
	Close() error
}

// ConnectFunc opens a tool execution environment for a harness session.
// Implementations do not own the model or the conversation.
type ConnectFunc func(context.Context, Target, string) (Runtime, error)

func (t Target) Identity() string {
	data, _ := json.Marshal(t)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (t Target) Label() string {
	switch t.Kind {
	case "docker", "container":
		return locales.WorkspaceDocker() + " · " + t.Directory
	case "remote":
		return locales.WorkspaceRemote() + " · " + t.Host
	case "microvm":
		return locales.WorkspaceMicroVM() + " · " + t.Host
	case "browser":
		return locales.WorkspaceBrowser()
	case "desktop":
		return locales.WorkspaceDesktop()
	default:
		if t.Kind == "" {
			return locales.WorkspaceLocal()
		}
		if t.Resource != "" {
			return t.Kind + " · " + t.Resource
		}
		return t.Kind
	}
}

// NewConnector builds the host's execution environment dispatcher. Built-in
// kinds are reserved. Additional connectors validate their own target fields.
func NewConnector(additional map[string]ConnectFunc) (ConnectFunc, error) {
	connectors := map[string]ConnectFunc{
		"docker": connectContainer, "container": connectContainer, "remote": connectSSH,
		"browser": connectBrowser, "desktop": connectDesktop,
		"microvm": connectMicroVM,
	}
	for kind, connect := range additional {
		if kind == "" || kind == "local" || kind == "invalid" || strings.TrimSpace(kind) != kind || connectors[kind] != nil || connect == nil {
			return nil, fmt.Errorf("invalid or reserved execution environment kind %q", kind)
		}
		connectors[kind] = connect
	}
	return func(ctx context.Context, target Target, secret string) (Runtime, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		connect := connectors[target.Kind]
		if connect == nil {
			return nil, fmt.Errorf("不支持的工作区类型 %q", target.Kind)
		}
		runtime, err := connect(ctx, target, secret)
		if err == nil && runtime == nil {
			return nil, fmt.Errorf("execution environment connector %q returned no runtime", target.Kind)
		}
		return runtime, err
	}, nil
}

var defaultConnector, _ = NewConnector(nil)

func Open(ctx context.Context, target Target, secret string) (Runtime, error) {
	return defaultConnector(ctx, target, secret)
}

func connectContainer(ctx context.Context, target Target, _ string) (Runtime, error) {
	if strings.TrimSpace(target.Directory) == "" {
		return nil, errors.New("请选择工作目录")
	}
	rt, err := openContainer(ctx, target)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func connectSSH(ctx context.Context, target Target, secret string) (Runtime, error) {
	if strings.TrimSpace(target.Directory) == "" {
		return nil, errors.New("请选择工作目录")
	}
	rt, err := openSSH(ctx, target, secret)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// The VM manager owns the guest lifecycle. The guest's pinned SSH endpoint
// supplies file and command operations with the original MicroVM target ID.
func connectMicroVM(ctx context.Context, target Target, secret string) (Runtime, error) {
	rt, err := connectSSH(ctx, target, secret)
	if err != nil {
		return nil, err
	}
	if rt.Environment().OS != "linux" {
		_ = rt.Close()
		return nil, errors.New(locales.WorkspaceMicroVMLinuxRequired())
	}
	return rt, nil
}

type expectedWrite struct{ Path, Version string }
type expectedWriteKey struct{}

func WithWriteVersion(ctx context.Context, path, version string) context.Context {
	return context.WithValue(ctx, expectedWriteKey{}, expectedWrite{path, version})
}

func writeVersion(ctx context.Context, target ResolvedFileSystem, name string) (string, error) {
	resolved, version, err := target.Resolve(ctx, name, true)
	if err != nil {
		return "", err
	}
	if expected, ok := ctx.Value(expectedWriteKey{}).(expectedWrite); ok && (resolved != expected.Path || version != expected.Version) {
		return "", errors.New("文件已变化，请重新请求批准")
	}
	if resolved != name {
		return "", errors.New("文件路径已变化")
	}
	return version, nil
}
