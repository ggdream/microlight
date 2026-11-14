package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
)

type Container struct{ ID, Name string }

type ContainerEngine struct {
	Name       string
	Connection string
}

type containerCLI struct {
	executable string
	prefix     []string
}

func ContainerEngines() []string {
	var engines []string
	for _, name := range []string{"docker", "podman", "nerdctl"} {
		if _, err := containerExecutable(name); err == nil {
			engines = append(engines, name)
		}
	}
	return engines
}

func containerExecutable(name string) (string, error) {
	switch name {
	case "docker", "podman", "nerdctl":
	default:
		return "", errors.New(locales.WorkspaceContainerEngineMissing())
	}
	if cli, err := exec.LookPath(name); err == nil {
		return cli, nil
	}
	if runtime.GOOS == "darwin" {
		for _, dir := range []string{"/usr/local/bin/", "/opt/homebrew/bin/", "/Applications/Docker.app/Contents/Resources/bin/"} {
			if info, err := os.Stat(dir + name); err == nil && info.Mode().IsRegular() {
				return dir + name, nil
			}
		}
	}
	return "", errors.New(locales.WorkspaceContainerEngineMissing())
}

func ListContainers(ctx context.Context, name string) (ContainerEngine, []Container, error) {
	engine := ContainerEngine{Name: name}
	executable, err := containerExecutable(name)
	if err != nil {
		return engine, nil, err
	}
	cli := containerCLI{executable: executable}
	switch name {
	case "docker":
		out, err := cli.call(ctx, "context", "show")
		if err != nil {
			return engine, nil, err
		}
		engine.Connection = strings.TrimSpace(out)
	case "podman":
		if os.Getenv("CONTAINER_HOST") != "" {
			return engine, nil, errors.New(locales.WorkspaceContainerLocalOnly())
		}
		connection, err := cli.podmanConnection(ctx, os.Getenv("CONTAINER_CONNECTION"))
		if err != nil {
			return engine, nil, err
		}
		engine.Connection = connection
	case "nerdctl":
		engine.Connection = "default"
	}
	cli, err = openContainerCLI(ctx, Target{Engine: engine})
	if err != nil {
		return engine, nil, err
	}
	out, err := cli.call(ctx, "ps", "--no-trunc", "--format", "{{.ID}}\t{{.Names}}")
	if err != nil {
		return engine, nil, err
	}
	var containers []Container
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if id, label, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			containers = append(containers, Container{ID: id, Name: label})
		}
	}
	return engine, containers, nil
}

func openContainerCLI(ctx context.Context, target Target) (containerCLI, error) {
	engine := target.Engine
	if engine.Name == "" && target.Kind == "docker" {
		engine = ContainerEngine{Name: "docker", Connection: target.DockerContext}
	}
	executable, err := containerExecutable(engine.Name)
	if err != nil {
		return containerCLI{}, err
	}
	cli := containerCLI{executable: executable}
	switch engine.Name {
	case "docker":
		if engine.Connection == "" {
			return cli, errors.New(locales.WorkspaceContainerEngineMissing())
		}
		endpoint, err := cli.call(ctx, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}", "--", engine.Connection)
		if err != nil {
			return cli, err
		}
		endpoint = strings.TrimSpace(endpoint)
		if !strings.HasPrefix(endpoint, "unix:///") && !strings.HasPrefix(endpoint, "npipe:////./pipe/") {
			return cli, errors.New(locales.WorkspaceContainerLocalOnly())
		}
		cli.prefix = []string{"--context", engine.Connection}
	case "podman":
		if engine.Connection == "" {
			cli.prefix = []string{"--remote=false"}
		} else {
			if _, err := cli.podmanConnection(ctx, engine.Connection); err != nil {
				return cli, err
			}
			cli.prefix = []string{"--connection", engine.Connection}
		}
	case "nerdctl":
		cli.prefix = []string{"--namespace", engine.Connection}
	}
	return cli, nil
}

func (cli containerCLI) podmanConnection(ctx context.Context, name string) (string, error) {
	out, err := cli.call(ctx, "system", "connection", "list", "--format", "json")
	if err != nil {
		return "", err
	}
	var connections []struct {
		Name, URI          string
		Default, IsMachine bool
	}
	if err := json.Unmarshal([]byte(out), &connections); err != nil {
		return "", err
	}
	for _, connection := range connections {
		if (name != "" && connection.Name != name) || (name == "" && !connection.Default) {
			continue
		}
		u, err := url.Parse(connection.URI)
		if err == nil && ((u.Scheme == "unix" && u.Host == "" && strings.HasPrefix(u.Path, "/")) ||
			(connection.IsMachine && u.Scheme == "ssh" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"))) {
			return connection.Name, nil
		}
		return "", errors.New(locales.WorkspaceContainerLocalOnly())
	}
	if name != "" || runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return "", errors.New(locales.WorkspaceContainerEngineMissing())
	}
	return "", nil
}

func (cli containerCLI) run(ctx context.Context, stdin io.Reader, args ...string) (string, string, bool, error) {
	return runCLI(ctx, stdin, cli.executable, append(append([]string(nil), cli.prefix...), args...)...)
}

func (cli containerCLI) call(ctx context.Context, args ...string) (string, error) {
	out, stderr, truncated, err := cli.run(ctx, nil, args...)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
	}
	if truncated {
		return "", errors.New(locales.WorkspaceContainerOutputLarge())
	}
	return out, nil
}
