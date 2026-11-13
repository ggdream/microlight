package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
)

func TestRunnerUsesScopedWorkspaceInstructions(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	workspace := filepath.Join(project, "selected")
	sibling := filepath.Join(project, "sibling")
	for _, dir := range []string{workspace, sibling} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(project, ".git"), "gitdir: worktree")
	write(filepath.Join(root, "AGENTS.md"), "Outside repository marker")
	write(filepath.Join(project, "AGENTS.md"), "Root instruction marker")
	write(filepath.Join(sibling, "AGENTS.md"), "Sibling instruction marker")
	write(filepath.Join(workspace, "AGENTS.override.md"), "Vendor override marker")
	selected := filepath.Join(workspace, "AGENTS.md")
	write(selected, "Original nested marker")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []chatRequestMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) < 2 {
			t.Errorf("invalid model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		system, last := request.Messages[0], request.Messages[len(request.Messages)-1]
		if system.Role != "system" || last.Role != "user" {
			t.Errorf("unexpected message roles: %s, %s", system.Role, last.Role)
		}
		for _, marker := range []string{"Outside repository marker", "Sibling instruction marker", "Vendor override marker"} {
			if strings.Contains(system.Content, marker) {
				t.Errorf("loaded out-of-scope instructions: %s", marker)
			}
		}
		switch last.Content {
		case "first":
			parent := strings.Index(system.Content, "Root instruction marker")
			child := strings.Index(system.Content, "Original nested marker")
			if parent < 0 || child <= parent {
				t.Error("expected root instructions before the selected directory's instructions")
			}
		case "follow-up":
			if !strings.Contains(system.Content, "Updated nested marker") || strings.Contains(system.Content, "Original nested marker") {
				t.Error("expected updated instructions instead of stale ones on follow-up")
			}
		case "no workspace":
			if strings.Contains(system.Content, "instruction marker") || strings.Contains(system.Content, "nested marker") {
				t.Error("project guidance leaked into a session without a workspace")
			}
		default:
			t.Errorf("unexpected last message: %q", last.Content)
		}
		writeTestStream(w, "OK")
	}))
	defer server.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: server.URL, Model: "test"})
	backend.skillsHome = ""
	ctx := localtools.WithAccess(t.Context(), localtools.NewAccess(workspace, localtools.Ask))
	run := func(ctx context.Context, id, input string) {
		t.Helper()
		if _, err := backend.runSession(ctx, store, id, []*turn{userTurn(input)}, nil, "", func(replyDelta) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	run(ctx, "scoped", "first")
	write(selected, "Updated nested marker")
	run(ctx, "scoped", "follow-up")
	run(t.Context(), "unscoped", "no workspace")
}
