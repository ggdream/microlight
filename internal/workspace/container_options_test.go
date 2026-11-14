package workspace

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDockerOptionsBuildArguments(t *testing.T) {
	source := t.TempDir()
	options := ContainerOptions{
		Name:   "dev-workspace",
		Env:    []ContainerVariable{{Name: "APP_CONFIG", Value: "one two=three"}},
		Ports:  []ContainerPort{{Host: "8080", Container: "80", Protocol: "tcp"}},
		Mounts: []ContainerMount{{Source: source, Target: "/data", ReadOnly: true}},
	}
	args, err := options.args("/workspace")
	want := []string{
		"--env", "APP_CONFIG=one two=three",
		"--publish", "127.0.0.1:8080:80/tcp",
		"--mount", "type=bind,source=" + source + ",target=/data,readonly",
	}
	if err != nil || !slices.Equal(args, want) {
		t.Fatalf("args = %q, err = %v, want %q", args, err, want)
	}
	options.Mounts[0].Target = "/data/../workspace"
	if _, err := options.args("/workspace"); err == nil {
		t.Fatal("accepted a mount that would replace the workspace")
	}
}

func TestDockerOptionsRoundTrip(t *testing.T) {
	if os.Getenv("MICROLIGHT_TEST_DOCKER") != "1" {
		t.Skip("set MICROLIGHT_TEST_DOCKER to test with Docker")
	}
	ctx := t.Context()
	engine, _, err := ListContainers(ctx, "docker")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "marker"), []byte("readable"), 0600); err != nil {
		t.Fatal(err)
	}
	id := "options-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	target := Target{Kind: "container", Directory: "/workspace", Engine: engine}
	t.Cleanup(func() { CleanupContainer(target, id, true) })
	options := ContainerOptions{
		Name:   "microlight-test-" + id,
		Env:    []ContainerVariable{{Name: "APP_CONFIG", Value: "one two=three"}},
		Ports:  []ContainerPort{{Host: port, Container: "8080", Protocol: "tcp"}},
		Mounts: []ContainerMount{{Source: source, Target: "/data", ReadOnly: true}},
	}
	target, err = CreateContainer(ctx, target, "postgres:16-alpine", "", id, options)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := openContainer(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	name, err := runtime.engine.call(ctx, "inspect", "--format", "{{.Name}}", target.Container)
	if err != nil || strings.TrimSpace(name) != "/"+options.Name {
		t.Fatalf("container name = %q, %v", name, err)
	}
	mapping, err := runtime.engine.call(ctx, "port", target.Container, "8080/tcp")
	if err != nil || strings.TrimSpace(mapping) != "127.0.0.1:"+port {
		t.Fatalf("port mapping = %q, %v", mapping, err)
	}
	out, err := runtime.command(ctx, `test "$APP_CONFIG" = 'one two=three' && test "$(cat /data/marker)" = readable && ! ( touch /data/mutation ) && test -w /workspace && printf verified`, "")
	if err != nil || out != "verified" {
		t.Fatalf("container environment and mounts: %q, %v", out, err)
	}
}
