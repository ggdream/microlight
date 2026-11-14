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
}

type Environment struct {
	OS, Arch, Shell string
}

type FileSystem interface {
	LsInfo(context.Context, *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error)
	Read(context.Context, *filesystem.ReadRequest) (*filesystem.FileContent, error)
	Write(context.Context, *filesystem.WriteRequest) error
}

type Runtime interface {
	filesystem.Shell
	FileSystem
	Resolve(context.Context, string, bool) (path, version string, err error)
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
	default:
		return locales.WorkspaceLocal()
	}
}

func Open(ctx context.Context, target Target, secret string) (Runtime, error) {
	if strings.TrimSpace(target.Directory) == "" {
		return nil, errors.New("请选择工作目录")
	}
	switch target.Kind {
	case "docker", "container":
		return openContainer(ctx, target)
	case "remote":
		return openSSH(ctx, target, secret)
	default:
		return nil, errors.New("不支持的工作区类型")
	}
}

type expectedWrite struct{ Path, Version string }
type expectedWriteKey struct{}

func WithWriteVersion(ctx context.Context, path, version string) context.Context {
	return context.WithValue(ctx, expectedWriteKey{}, expectedWrite{path, version})
}

func writeVersion(ctx context.Context, target Runtime, name string) (string, error) {
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
