package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
)

func CreateContainer(ctx context.Context, target Target, image, source, id string, options ContainerOptions) (Target, error) {
	extra, err := options.args(target.Directory)
	if err != nil {
		return target, err
	}
	engine, err := openContainerCLI(ctx, target)
	if err != nil {
		return target, err
	}
	if !path.IsAbs(target.Directory) || strings.ContainsAny(target.Directory+source, ",\r\n") || strings.TrimSpace(image) == "" {
		return target, errors.New("需要镜像和有效的容器内绝对路径")
	}
	if source != "" {
		if !filepath.IsAbs(source) {
			return target, errors.New("请选择本机绝对路径")
		}
		info, err := os.Stat(source)
		if err != nil || !info.IsDir() {
			return target, errors.New("本机项目目录不可用")
		}
	}

	name := "microlight-" + id
	mount := "type=bind,source=" + source
	if source == "" {
		if _, err := engine.call(ctx, "volume", "create", "--label", "dev.local.microlight.workspace="+id, "--", name); err != nil {
			return target, err
		}
		mount = "type=volume,source=" + name
	}
	containerName := name
	if options.Name != "" {
		containerName = options.Name
	}
	args := []string{"run", "-d", "--name", containerName, "--label", "dev.local.microlight.workspace=" + id, "--mount", mount + ",target=" + target.Directory, "--workdir", target.Directory, "--entrypoint", "/bin/sh"}
	args = append(args, extra...)
	args = append(args, "--", image, "-c", "while :; do sleep 3600; done")
	result, err := engine.call(ctx, args...)
	if err != nil {
		return target, err
	}
	target.Container, target.Managed = strings.TrimSpace(result), true
	return target, nil
}

type containerRuntime struct {
	posixFiles
	target Target
	engine containerCLI
	env    Environment
	jobs   bool
}

func openContainer(ctx context.Context, target Target) (*containerRuntime, error) {
	if !path.IsAbs(target.Directory) || target.Container == "" {
		return nil, errors.New("请选择容器和容器内绝对路径")
	}
	engine, err := openContainerCLI(ctx, target)
	if err != nil {
		return nil, err
	}
	d := &containerRuntime{target: target, engine: engine, env: Environment{OS: "linux", Shell: "/bin/sh"}}
	status, err := engine.call(ctx, "inspect", "--type", "container", "--format", "{{.Id}} {{.State.Running}}", "--", target.Container)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(status)
	if len(fields) != 2 || fields[0] != target.Container {
		return nil, errors.New("容器身份已变化")
	}
	if fields[1] != "true" {
		if !target.Managed {
			return nil, errors.New("所选容器未运行")
		}
		if _, err := engine.call(ctx, "start", "--", target.Container); err != nil {
			return nil, err
		}
	}
	d.posixFiles = posixFiles{directory: target.Directory, command: d.command}
	out, err := d.command(ctx, posixFileProbe+"; uname -sm", "")
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(out)
	if len(parts) < 2 || parts[0] != "Linux" {
		return nil, errors.New("容器需要 Linux 和 /bin/sh")
	}
	d.env.Arch = parts[1]
	d.jobs = supportsRemoteCommands(ctx, d.runCommandControl)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *containerRuntime) Commands() backgroundshell.RecoverableShell {
	if !d.jobs {
		return nil
	}
	return remoteCommands{run: d.runCommandControl, directory: d.target.Directory, scope: d.target.Identity()}
}

func (d *containerRuntime) runCommandControl(ctx context.Context, script string) (string, error) {
	return d.command(ctx, script, "")
}

func (d *containerRuntime) Environment() Environment { return d.env }
func (d *containerRuntime) Close() error             { return nil }
func (d *containerRuntime) args(script string) []string {
	return []string{"exec", "-i", "-w", d.target.Directory, d.target.Container, "/bin/sh", "-c", script}
}
func (d *containerRuntime) command(ctx context.Context, script, input string) (string, error) {
	out, stderr, truncated, err := d.engine.run(ctx, strings.NewReader(input), d.args(script)...)
	if err != nil {
		return "", fmt.Errorf("容器操作失败: %w %s", err, strings.TrimSpace(stderr))
	}
	if truncated {
		return "", errors.New("容器响应过大")
	}
	return out, nil
}

func (d *containerRuntime) Execute(ctx context.Context, req *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	deadline, cancel := commandContext(ctx, req)
	defer cancel()
	out, stderr, truncated, err := d.engine.run(deadline, nil, d.args(req.Command)...)
	if deadline.Err() != nil {
		return nil, fmt.Errorf("容器命令结果未知: %w", deadline.Err())
	}
	return shellResult(out+stderr, truncated, err)
}

func CleanupContainer(target Target, id string, volume bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	engine, err := openContainerCLI(ctx, target)
	if err != nil {
		return
	}
	name := "microlight-" + id
	containers, err := engine.call(ctx, "ps", "-aq", "--no-trunc", "--filter", "label=dev.local.microlight.workspace="+id)
	if err == nil {
		for _, container := range strings.Fields(containers) {
			_, _ = engine.call(ctx, "rm", "-f", "--", container)
		}
	}
	if volume {
		label, err := engine.call(ctx, "volume", "inspect", "--format", "{{index .Labels \"dev.local.microlight.workspace\"}}", "--", name)
		if err == nil && strings.TrimSpace(label) == id {
			_, _ = engine.call(ctx, "volume", "rm", "--", name)
		}
	}
}
