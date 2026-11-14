package localtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/ggdream/microlight/internal/workspace"
)

func TestReconnectAfterLostCommandLaunch(t *testing.T) {
	for _, tc := range []struct {
		name              string
		accepted, missing bool
		wantStarts        int
	}{
		{name: "accepted", accepted: true, wantStarts: 1},
		{name: "authoritatively missing", missing: true, wantStarts: 2},
		{name: "proxy returned not found", wantStarts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var id string
			starts, observations := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch req.URL.Path {
				case "/v1/desktop/info":
					_, _ = w.Write([]byte(`{"SessionID":"executor","Files":true,"Commands":true}`))
				case "/v1/desktop/files/resolve":
					_, _ = w.Write([]byte(`{"Path":"/","IsDir":true}`))
				case "/v1/desktop/commands/start", "/v1/desktop/commands/status":
					var args struct{ ID string }
					if err := json.NewDecoder(req.Body).Decode(&args); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if id == "" {
						id = args.ID
					}
					if args.ID != id {
						t.Errorf("task ID changed: %q → %q", id, args.ID)
					}
					if req.URL.Path == "/v1/desktop/commands/start" {
						starts++
						if starts == 1 {
							conn, _, err := w.(http.Hijacker).Hijack()
							if err != nil {
								t.Error(err)
								return
							}
							_ = conn.Close()
							return
						}
					} else {
						observations++
						if !tc.accepted && starts == 1 {
							detail := "route not found"
							if tc.missing {
								detail = "command-task-not-found"
							}
							http.Error(w, detail, http.StatusNotFound)
							return
						}
					}
					_ = json.NewEncoder(w).Encode(backgroundtool.Outcome{
						Status: backgroundtask.StatusCompleted,
						Data:   json.RawMessage(`{"Output":"finished"}`),
					})
				default:
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			target := workspace.Target{Kind: "desktop", Resource: server.URL, Directory: "/"}
			ctx := WithWorkspaceReconnect(t.Context(), func(ctx context.Context) (workspace.Runtime, error) {
				return workspace.Open(ctx, target, "desktop-test-token-00000000000000")
			})
			runtime, err := workspace.Open(ctx, target, "desktop-test-token-00000000000000")
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			ops, err := workspace.DiscoverOperations(ctx, runtime)
			if err != nil || ops.Commands == nil {
				t.Fatalf("command capability unavailable: %v", err)
			}
			response, err := executeRecoverable(ctx, ops.Commands, localOperation{callID: "test", content: "echo finished"})
			if !tc.accepted && !tc.missing {
				if err == nil {
					t.Fatal("expected unresolved transport error")
				}
			} else if err != nil || response.Output != "finished" {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if starts != tc.wantStarts || observations != 1 {
				t.Fatalf("starts = %d, observations = %d", starts, observations)
			}
		})
	}
}
