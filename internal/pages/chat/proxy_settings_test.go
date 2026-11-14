package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/skills"
	"github.com/ggdream/microlight/internal/workspace"
	"github.com/ggdream/microlight/pkg/proxy"

	"github.com/cloudwego/eino/components/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCloudWorkspaceUsesConfiguredProxy(t *testing.T) {
	var connects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "api.e2b.app:443" {
			t.Errorf("unexpected proxy request %s %s", r.Method, r.Host)
		}
		if r.Header.Get("X-API-Key") != "" {
			t.Error("sandbox key leaked to the CONNECT proxy")
		}
		connects.Add(1)
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer server.Close()
	app := &chatApp{settings: modelSettings{Proxy: proxy.Config{Mode: proxy.Custom, URL: server.URL}}}
	open := app.workspaceConnector()
	// An in-flight operation and its reconnect use their original settings.
	app.settings.Proxy = proxy.Config{Mode: proxy.Direct}
	target := workspace.Target{Kind: "e2b", Resource: "proxy-probe", Directory: "/home/user"}
	for range 2 {
		rt, err := open(t.Context(), target, "test-key")
		if rt != nil {
			rt.Close()
		}
		if !errors.Is(err, proxy.ErrProxy) {
			t.Fatalf("expected a proxy error, got %v", err)
		}
	}
	if got := connects.Load(); got != 2 {
		t.Fatalf("proxy received %d CONNECT requests, want 2", got)
	}
}

func TestSharedProxyForModelMCPAndWebFetch(t *testing.T) {
	mcpFixture := mcp.NewServer(&mcp.Implementation{Name: "proxy-test", Version: "1"}, nil)
	mcp.AddTool(mcpFixture, &mcp.Tool{Name: "lookup", Description: "Test proxy routing"},
		func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "proxied"}}}, nil, nil
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpFixture }, &mcp.StreamableHTTPOptions{DisableLocalhostProtection: true})
	var modelCalls, mcpCalls, webCalls atomic.Int32
	var fetchRequested atomic.Bool
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() {
			t.Error("expected a forward-proxy request")
		}
		switch r.URL.Hostname() {
		case "model.example.invalid":
			modelCalls.Add(1)
			if fetchRequested.CompareAndSwap(true, false) {
				args, _ := json.Marshal(map[string]string{"url": "http://web.example.invalid/page"})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"web-1\",\"type\":\"function\",\"function\":{\"name\":\"fetch_web_page\",\"arguments\":%q}}]}}]}\n\ndata: [DONE]\n\n", args)
				return
			}
			writeTestStream(w, "OK")
		case "mcp.example.invalid":
			mcpCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer server-token" {
				t.Error("MCP server auth header missing")
			}
			mcpHandler.ServeHTTP(w, r)
		case "web.example.invalid":
			webCalls.Add(1)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "proxied page")
		default:
			http.Error(w, "unexpected destination", http.StatusBadRequest)
		}
	}))
	defer proxyServer.Close()

	dir := t.TempDir()
	store, err := openLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	defaultProxy, err := store.loadProxy()
	if err != nil || defaultProxy.Mode != proxy.System {
		t.Fatalf("default proxy = %v, error = %v", defaultProxy, err)
	}
	cfg := modelSettings{
		Protocol: openAIChat, BaseURL: "http://model.example.invalid/v1", Model: "test",
		Proxy: proxy.Config{Mode: proxy.Custom, URL: proxyServer.URL},
		MCPServers: []mcptools.Server{{
			Name: "test", URL: "http://mcp.example.invalid/mcp",
			Headers: []mcptools.Header{{Name: "Authorization", Value: "Bearer server-token"}},
		}},
	}
	if err := store.saveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	app := &chatApp{store: store}
	app.loadSettings()
	if app.settings.Proxy != cfg.Proxy {
		t.Fatalf("saved proxy = %v, want %v", app.settings.Proxy, cfg.Proxy)
	}
	app.openSettings()
	if err := checkConnection(t.Context(), app.connectionConfig()); err != nil {
		t.Fatal(err)
	}
	if err := app.backend.stream(t.Context(), []message{{Role: "user", Content: "Test chat"}}, func(replyDelta) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if modelCalls.Load() != 2 {
		t.Fatalf("expected connection test and chat via proxy, got %d requests", modelCalls.Load())
	}
	fetchRequested.Store(true)
	ctx := localtools.WithAccess(t.Context(), localtools.NewAccess("", localtools.Full))
	var reply turn
	pending, err := app.backend.runSession(ctx, store, newID(), []*turn{userTurn("Read the page")}, nil, "", func(delta replyDelta) error {
		reply.appendDelta(delta)
		return nil
	})
	if err != nil || pending != nil || webCalls.Load() != 1 || len(reply.Activity) == 0 ||
		reply.Activity[0].Output == nil || !strings.Contains(*reply.Activity[0].Output, "proxied page") {
		t.Fatalf("web fetch via app proxy: requests=%d, pending=%v, err=%v, activity=%+v", webCalls.Load(), pending, err, reply.Activity)
	}

	var ext agentExtensions
	defer ext.close()
	if err := ext.load(t.Context(), app.settings, &skills.Catalog{}, app.backend.network); err != nil {
		t.Fatal(err)
	}
	if len(ext.tools) != 1 {
		t.Fatalf("expected one MCP tool, got %d", len(ext.tools))
	}
	result, err := ext.tools[0].(tool.InvokableTool).InvokableRun(t.Context(), "{}")
	if err != nil || !strings.Contains(result, "proxied") || mcpCalls.Load() < 3 {
		t.Fatalf("proxied MCP invocation failed: %q, %v", result, err)
	}

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "direct")
	}))
	defer local.Close()
	response, err := (&http.Client{Transport: app.backend.network}).Get(local.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "direct" {
		t.Fatalf("loopback bypass failed: %q, %v", body, err)
	}
}
