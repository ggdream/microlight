package workspace

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
)

type ContainerOptions struct {
	Name   string
	Env    []ContainerVariable
	Ports  []ContainerPort
	Mounts []ContainerMount
}

type ContainerVariable struct{ Name, Value string }
type ContainerPort struct{ Host, Container, Protocol string }
type ContainerMount struct {
	Source, Target string
	ReadOnly       bool
}

var dockerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var dockerEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (options ContainerOptions) args(workdir string) ([]string, error) {
	if options.Name != "" && !dockerName.MatchString(options.Name) {
		return nil, errors.New(locales.WorkspaceDockerInvalidName())
	}
	var args []string
	names := make(map[string]bool)
	for _, variable := range options.Env {
		name := strings.TrimSpace(variable.Name)
		if !dockerEnvName.MatchString(name) || names[name] || strings.ContainsRune(variable.Value, 0) {
			return nil, errors.New(locales.WorkspaceDockerInvalidEnv())
		}
		names[name] = true
		args = append(args, "--env", name+"="+variable.Value)
	}
	ports := make(map[string]bool)
	for _, port := range options.Ports {
		host, hostErr := strconv.Atoi(strings.TrimSpace(port.Host))
		container, containerErr := strconv.Atoi(strings.TrimSpace(port.Container))
		key := strconv.Itoa(host) + "/" + port.Protocol
		if hostErr != nil || containerErr != nil || host < 1 || host > 65535 || container < 1 || container > 65535 ||
			(port.Protocol != "tcp" && port.Protocol != "udp") || ports[key] {
			return nil, errors.New(locales.WorkspaceDockerInvalidPort())
		}
		ports[key] = true
		args = append(args, "--publish", "127.0.0.1:"+strconv.Itoa(host)+":"+strconv.Itoa(container)+"/"+port.Protocol)
	}
	targets := map[string]bool{path.Clean(workdir): true}
	for _, mount := range options.Mounts {
		source, target := strings.TrimSpace(mount.Source), strings.TrimSpace(mount.Target)
		info, err := os.Stat(source)
		cleanTarget := path.Clean(target)
		if !filepath.IsAbs(source) || !path.IsAbs(target) || cleanTarget == "/" || targets[cleanTarget] ||
			strings.ContainsAny(source+target, ",\"\r\n\x00") || err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
			return nil, errors.New(locales.WorkspaceDockerInvalidMount())
		}
		targets[cleanTarget] = true
		value := "type=bind,source=" + source + ",target=" + cleanTarget
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}
	return args, nil
}
