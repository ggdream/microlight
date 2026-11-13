package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/skills"

	"github.com/cloudwego/eino/components/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAgentUsesConfiguredSkillsAndMCPWithApproval(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Test skill\n---\nSkill instructions loaded.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "lookup", Description: "Look up a test value"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		calls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "MCP-OK"}}}, nil, nil
	})
	endpoint := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer endpoint.Close()
	mcpTool := mcptools.ToolName("test", "lookup")

	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role, Content string }
			Tools    []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		hasTool := func(name string) bool {
			return slices.ContainsFunc(request.Tools, func(tool struct{ Function struct{ Name string } }) bool {
				return tool.Function.Name == name
			})
		}
		if !hasTool("skill") || !hasTool(mcpTool) {
			t.Errorf("missing configured tool: %+v", request.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		call := func(id, tool, args string) {
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":%q,\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n", id, tool, args)
		}
		i := len(request.Messages) - 1
		for i > 0 && request.Messages[i].Role == "system" {
			i--
		}
		last := request.Messages[i]
		switch {
		case last.Role == "user":
			call("skill-call", "skill", `{"skill":"demo"}`)
		case last.Role == "tool" && strings.Contains(last.Content, "Skill instructions loaded."):
			call("mcp-call", mcpTool, "{}")
		case last.Role == "tool" && strings.Contains(last.Content, "MCP-OK"):
			writeTestStream(w, "Done")
		default:
			t.Errorf("unexpected last message: %+v", last)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer model.Close()

	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config := modelSettings{
		BaseURL: model.URL, Model: "test",
		MCPServers: []mcptools.Server{{Name: "test", URL: endpoint.URL}},
	}
	if err := store.saveSettings(config); err != nil {
		t.Fatal(err)
	}
	saved, err := store.loadSettings()
	if err != nil || saved == nil || !reflect.DeepEqual(saved.MCPServers, config.MCPServers) {
		t.Fatalf("configuration did not round trip: %v", err)
	}
	newBackend := func() *Chat {
		backend := newChat(*saved)
		backend.skillsHome = dir
		return backend
	}
	question, err := newBackend().runSession(t.Context(), store, "test", []*turn{userTurn("Use both tools")}, nil, "", func(replyDelta) error { return nil })
	if err != nil || question == nil || question.Approval == nil || question.Approval.Target != endpoint.URL || calls.Load() != 0 {
		t.Fatalf("expected approval before tool execution, got %+v, %v, calls=%d", question, err, calls.Load())
	}
	var body strings.Builder
	pending, err := newBackend().runSession(t.Context(), store, "test", nil, question, localtools.AllowOnce, func(part replyDelta) error {
		body.WriteString(part.text)
		return nil
	})
	if err != nil || pending != nil || body.String() != "Done" || calls.Load() != 1 {
		t.Fatalf("MCP resume failed: %q, %+v, %v, calls=%d", body.String(), pending, err, calls.Load())
	}
}

func TestMCPStdio(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := modelSettings{
		BaseURL: defaultBaseURL, Model: defaultModel,
		MCPServers: []mcptools.Server{{
			Name: "stdio-test", Transport: mcptools.Stdio, Command: executable,
			Args: []string{"-test.run=^TestMCPStdioServerProcess$", "--", "an argument with spaces"},
		}},
	}
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.saveSettings(config); err != nil {
		t.Fatal(err)
	}
	saved, err := store.loadSettings()
	if err != nil || saved == nil || !reflect.DeepEqual(saved.MCPServers, config.MCPServers) {
		t.Fatalf("Stdio configuration did not round trip: %v", err)
	}
	var ext agentExtensions
	defer ext.close()
	if err := ext.load(t.Context(), *saved, &skills.Catalog{}, http.DefaultTransport); err != nil {
		t.Fatal(err)
	}
	if len(ext.tools) != 1 {
		t.Fatalf("expected one Stdio tool, got %d", len(ext.tools))
	}
	result, err := ext.tools[0].(tool.InvokableTool).InvokableRun(t.Context(), "{}")
	if err != nil || !strings.Contains(result, "an argument with spaces") {
		t.Fatalf("Stdio invocation failed: %q, %v", result, err)
	}
}

func TestMCPStdioServerProcess(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test-stdio", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo_arg", Description: "Echo the server argument"},
		func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: os.Args[len(os.Args)-1]}}}, nil, nil
		})
	if err := server.Run(t.Context(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestMCPHTTPHeaders(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-http", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "lookup", Description: "Test authenticated requests"},
		func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "authenticated"}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-API-Key") != "test-key" {
			t.Error("MCP request did not carry the configured headers")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	defer endpoint.Close()

	config := modelSettings{
		BaseURL: defaultBaseURL, Model: defaultModel,
		MCPServers: []mcptools.Server{{
			Name: "authenticated", Transport: mcptools.HTTP, URL: endpoint.URL,
			Headers: []mcptools.Header{{Name: "authorization", Value: "Bearer test-token"}, {Name: "X-API-Key", Value: "test-key"}},
		}},
	}
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.saveSettings(config); err != nil {
		t.Fatal(err)
	}
	saved, err := store.loadSettings()
	if err != nil || saved == nil || !reflect.DeepEqual(saved.MCPServers, config.MCPServers) {
		t.Fatalf("HTTP header configuration did not round trip: %v", err)
	}
	var ext agentExtensions
	if err := ext.load(t.Context(), *saved, &skills.Catalog{}, http.DefaultTransport); err != nil {
		ext.close()
		t.Fatal(err)
	}
	defer ext.close()
	if len(ext.tools) != 1 {
		t.Fatalf("expected one authenticated tool, got %d", len(ext.tools))
	}
	result, err := ext.tools[0].(tool.InvokableTool).InvokableRun(t.Context(), "{}")
	if err != nil || !strings.Contains(result, "authenticated") || requests.Load() < 3 {
		t.Fatalf("authenticated call failed: %q, %v, requests=%d", result, err, requests.Load())
	}
}
