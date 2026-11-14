package localtools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ggdream/microlight/internal/workspace"
)

func TestRemoteWorkspaceDoesNotUseHostProjectPath(t *testing.T) {
	host := t.TempDir()
	ctx := WithRuntime(t.Context(), NewAccess(host, Ask), nil, workspace.Target{
		Kind: "remote", Directory: "/work",
	}, workspace.Operations{})
	if path := Workspace(ctx, host); path != "" {
		t.Fatalf("remote session exposed a host project path: %q", path)
	}
}

func TestPermissionModesKeepSymlinkEscapesAndOverwritesBehindApproval(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "outside-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "existing.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, args string
		ask, smart bool
	}{
		{Read, `{"path":"outside-link"}`, true, true},
		{Read, `{"path":"existing.txt"}`, false, false},
		{Write, `{"path":"new.txt","content":"x"}`, true, false},
		{Write, `{"path":"existing.txt","content":"x"}`, true, true},
		{Write, `{"path":".env","content":"x"}`, true, true},
		{Fetch, `{"url":"https://example.com/"}`, true, true},
	} {
		handler := Middleware{Workspace: workspace}
		op, err := handler.resolve(tc.name, tc.args)
		if err != nil {
			t.Fatal(err)
		}
		for mode, want := range map[Mode]bool{Ask: tc.ask, Smart: tc.smart, Full: false} {
			handler.Mode = mode
			if got := handler.needsApproval(op); got != want {
				t.Errorf("%s %s in %s: approval = %t, want %t", tc.name, tc.args, mode, got, want)
			}
		}
	}
}
