package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
)

func TestWorkspaceTargetPersistsWithConversation(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, target := range []workspace.Target{
		{Kind: "remote", Directory: "/work", Host: "example.invalid", Port: "22",
			User: "test", Auth: "password", HostKey: "SHA256:fixture", Credential: "credential-reference"},
		{Kind: "docker", Directory: "/work", DockerContext: "legacy-context", Container: "legacy-id"},
		{Kind: "container", Directory: "/work", Engine: workspace.ContainerEngine{Name: "podman", Connection: "local"}, Container: "podman-id"},
		{Kind: "container", Directory: "/work", Engine: workspace.ContainerEngine{Name: "nerdctl", Connection: "default"}, Container: "containerd-id"},
	} {
		chat := &conversation{ID: newID(), Title: "Workspace", target: target}
		if err := store.saveHistory(history{Selected: chat.ID, Chats: []*conversation{chat}}, chat); err != nil {
			t.Fatal(err)
		}
		loaded, err := store.loadHistory()
		if err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(loaded.Chats, func(saved *conversation) bool { return saved.ID == chat.ID })
		if index < 0 || loaded.Chats[index].target != target || loaded.Chats[index].workspace != "" {
			t.Fatalf("workspace conversation %s did not restore target %+v", chat.ID, target)
		}
	}
}

func TestWorkspaceRuntimeRoundTrip(t *testing.T) {
	var targets []workspace.Target
	if os.Getenv("MICROLIGHT_TEST_DOCKER") == "1" {
		targets = append(targets, workspace.Target{Kind: "docker", Directory: "/workspace"})
	}
	if key := os.Getenv("MICROLIGHT_TEST_SSH_KEY"); key != "" {
		targets = append(targets, workspace.Target{Kind: "remote", Host: "127.0.0.1", Port: "42173", User: "root", Auth: "key", KeyFile: key, Directory: os.Getenv("MICROLIGHT_TEST_SSH_DIR")})
	}
	if len(targets) == 0 {
		t.Skip("set MICROLIGHT_TEST_DOCKER or MICROLIGHT_TEST_SSH_KEY to run against disposable workspaces")
	}
	for _, target := range targets {
		t.Run(target.Kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			if target.Kind == "docker" {
				engine, _, err := workspace.ListContainers(ctx, "docker")
				if err != nil {
					t.Fatal(err)
				}
				target.Engine = engine
				id := newID()
				t.Cleanup(func() { workspace.CleanupContainer(target, id, true) })
				target, err = workspace.CreateContainer(ctx, target, "postgres:16-alpine", "", id, workspace.ContainerOptions{})
				if err != nil {
					t.Fatal(err)
				}
				_, entries, err := workspace.ListContainers(ctx, "docker")
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range entries {
					found = found || item.ID == target.Container
				}
				if !found {
					t.Fatal("created container is missing from the running container selector")
				}
			} else {
				_, err := workspace.Open(ctx, target, "")
				var unknown *workspace.UnknownHostKey
				if !errors.As(err, &unknown) {
					t.Fatalf("expected host fingerprint, got %v", err)
				}
				target.HostKey = unknown.Fingerprint
			}

			host := t.TempDir()
			filename := "proof-" + newID() + ".txt"
			hostFile := filepath.Join(host, filename)
			if err := os.WriteFile(hostFile, []byte("host sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			toolName := localtools.Write
			command := "pwd; printf '%s' command > " + filename
			arguments := map[string]string{"path": filename, "content": "first"}
			var toolResponse string
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role    string
						Content json.RawMessage
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
					t.Errorf("invalid model request: %v", err)
					w.WriteHeader(400)
					return
				}
				last := request.Messages[len(request.Messages)-1]
				if last.Role == "tool" {
					toolResponse = string(last.Content)
					writeTestStream(w, "Finished")
					return
				}
				args, _ := json.Marshal(arguments)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"workspace-tool\",\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n", toolName, args)
			}))
			defer model.Close()
			store, err := openLocalStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			backend := newChat(modelSettings{Protocol: openAIChat, BaseURL: model.URL, Model: "test"})
			agent := PersonalAgent{backend: backend, target: target, access: localtools.NewAccess(host, localtools.Ask)}
			emit := func(replyDelta) error { return nil }
			read := func(want string) {
				t.Helper()
				rt, err := workspace.Open(ctx, target, "")
				if err != nil {
					t.Fatal(err)
				}
				defer rt.Close()
				files := rt.(workspace.ResolvedFileSystem)
				resolved, _, err := files.Resolve(ctx, filename, false)
				if want == "" {
					if err == nil {
						t.Fatal("file already exists on target")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				actual, err := files.Read(ctx, &filesystem.ReadRequest{FilePath: resolved})
				if err != nil || actual.Content != want {
					t.Fatalf("target file = %+v, %v; want %q", actual, err, want)
				}
			}
			start := func() (string, *clarification) {
				t.Helper()
				toolResponse = ""
				id := newID()
				q, err := agent.run(localtools.WithAccess(ctx, agent.access), store, id, []*turn{userTurn("Run tool")}, nil, "", emit)
				if err != nil {
					t.Fatal(err)
				}
				return id, q
			}
			resume := func(id string, q *clarification, answer string) {
				t.Helper()
				if q == nil || q.Approval == nil {
					t.Fatal("expected permission interrupt")
				}
				q, err := agent.run(localtools.WithAccess(ctx, agent.access), store, id, nil, q, answer, emit)
				if err != nil || q != nil {
					t.Fatalf("resume did not finish: %+v, %v", q, err)
				}
			}
			id, q := start()
			read("")
			resume(id, q, localtools.Deny)
			read("")
			id, q = start()
			read("")
			resume(id, q, localtools.AllowOnce)
			read("first")
			arguments["content"] = "overwrite"
			id, q = start()
			read("first")
			resume(id, q, localtools.AllowOnce)
			read("overwrite")
			arguments["content"] = "stale approval"
			id, q = start()
			func() {
				rt, err := workspace.Open(ctx, target, "")
				if err != nil {
					t.Fatal(err)
				}
				defer rt.Close()
				files := rt.(workspace.ResolvedFileSystem)
				resolved, _, err := files.Resolve(ctx, filename, true)
				if err != nil {
					t.Fatal(err)
				}
				if err := files.Write(ctx, &filesystem.WriteRequest{FilePath: resolved, Content: "external edit"}); err != nil {
					t.Fatal(err)
				}
			}()
			resume(id, q, localtools.AllowOnce)
			if !strings.Contains(toolResponse, "操作目标已变化") {
				t.Fatalf("stale approval was not rejected: %q", toolResponse)
			}
			read("external edit")
			toolName, arguments = localtools.Execute, map[string]string{"command": command}
			id, q = start()
			read("external edit")
			resume(id, q, localtools.AllowOnce)
			read("command")
			if !strings.Contains(toolResponse, target.Directory) {
				t.Fatalf("command reported wrong working directory: %q", toolResponse)
			}
			// The approved tool must go through the recoverable command path,
			// using the same checkpoint and call ID on resume.
			rt, err := workspace.Open(ctx, target, "")
			if err != nil {
				t.Fatal(err)
			}
			commands := rt.(workspace.CommandProvider).Commands()
			if commands == nil {
				rt.Close()
				t.Fatal("recoverable shell capability is missing")
			}
			_, recoverErr := commands.RecoverCommand(ctx, &backgroundshell.RecoverCommandRequest{
				TaskID: q.CheckpointID + "/workspace-tool", Command: command,
			})
			rt.Close()
			if recoverErr != nil {
				t.Fatalf("approved tool command cannot be recovered: %v", recoverErr)
			}
			t.Run("resume-original-turn", func(t *testing.T) {
				dbDir := t.TempDir()
				before, err := openLocalStore(dbDir)
				if err != nil {
					t.Fatal(err)
				}
				defer before.Close()
				arguments = map[string]string{"command": "printf x >> " + filename + "; cat " + filename}
				chat := &conversation{ID: newID(), sessionID: newID(), target: target}
				user := userTurn("Run the remote command")
				accessCtx := localtools.WithAccess(ctx, agent.access)
				approval, err := agent.run(accessCtx, before, chat.sessionID, []*turn{user}, nil, "", emit)
				if err != nil || approval == nil {
					t.Fatalf("approval = %+v, %v", approval, err)
				}
				approval.Answer = localtools.AllowOnce
				chat.Turns = []*turn{user, {message: message{Role: "assistant"}, State: "pending", Clarifications: []clarification{*approval}}}
				if err := before.saveHistory(history{Chats: []*conversation{chat}, Selected: chat.ID}, chat); err != nil {
					t.Fatal(err)
				}
				record := localtools.RemoteCommand{ID: approval.CheckpointID + "/workspace-tool", CallID: "workspace-tool", Command: arguments["command"]}
				if err := before.commandJournal(chat.ID, "", target.Identity(), 1).Record(ctx, record); err != nil {
					t.Fatal(err)
				}
				rt, err := workspace.Open(ctx, target, "")
				if err != nil {
					t.Fatal(err)
				}
				run, err := rt.(workspace.CommandProvider).Commands().StartCommand(ctx, &backgroundshell.StartCommandRequest{TaskID: record.ID, Command: record.Command, Attempt: 1})
				if err != nil {
					rt.Close()
					t.Fatal(err)
				}
				_, err = run.Wait(ctx)
				rt.Close()
				if err != nil {
					t.Fatal(err)
				}
				if err := before.Close(); err != nil {
					t.Fatal(err)
				}
				after, err := openLocalStore(dbDir)
				if err != nil {
					t.Fatal(err)
				}
				defer after.Close()
				app := &chatApp{store: after}
				app.loadHistory()
				restored := app.history.Chats[0]
				entry := restored.Turns[1]
				if entry.resumeCommand == nil || restored.sessionID != chat.sessionID {
					t.Fatal("original approval, command, or session was not restored")
				}
				entry.State = "pending"
				if err := after.saveHistory(app.history, restored); err != nil {
					t.Fatal(err)
				}
				resumeCtx := localtools.WithCommandJournal(accessCtx, after.commandJournal(chat.ID, "", target.Identity(), 1))
				resumeCtx = localtools.WithCommandReattachment(resumeCtx, entry.resumeCommand.RemoteCommand)
				approval, err = agent.run(resumeCtx, after, restored.sessionID, nil, &entry.Clarifications[0], localtools.AllowOnce, emit)
				if err != nil || approval != nil || !strings.Contains(toolResponse, "commandx") {
					t.Fatalf("original reply did not continue: %+v, %v, %q", approval, err, toolResponse)
				}
				read("commandx")
			})
			for _, tool := range []string{localtools.Read, localtools.List} {
				toolName = tool
				if tool == localtools.Read {
					arguments = map[string]string{"path": filename}
				} else {
					arguments = map[string]string{"path": "."}
				}
				_, q = start()
				if q != nil {
					t.Fatal("in-workspace read unexpectedly requested approval")
				}
				want := "commandx"
				if tool == localtools.List {
					want = filename
				}
				if !strings.Contains(toolResponse, want) {
					t.Fatalf("%s output = %q, want %q", tool, toolResponse, want)
				}
			}
			data, err := os.ReadFile(hostFile)
			if err != nil || string(data) != "host sentinel" {
				t.Fatalf("host sentinel changed: %q, %v", data, err)
			}
		})
	}
}
