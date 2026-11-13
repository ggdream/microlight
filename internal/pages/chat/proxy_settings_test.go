package chat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/skills"
	"github.com/ggdream/microlight/pkg/proxy"

	"github.com/cloudwego/eino/components/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSharedProxyForModelAndMCP(t *testing.T) {
	mcpFixture := mcp.NewServer(&mcp.Implementation{Name: "proxy-test", Version: "1"}, nil)
	mcp.AddTool(mcpFixture, &mcp.Tool{Name: "lookup", Description: "Test proxy routing"},
		func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "proxied"}}}, nil, nil
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpFixture }, &mcp.StreamableHTTPOptions{DisableLocalhostProtection: true})
	var modelCalls, mcpCalls atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() {
			t.Error("expected a forward-proxy request")
		}
		switch r.URL.Hostname() {
		case "model.example.invalid":
			modelCalls.Add(1)
			writeTestStream(w, "OK")
		case "mcp.example.invalid":
			mcpCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer server-token" {
				t.Error("MCP server auth header missing")
			}
			mcpHandler.ServeHTTP(w, r)
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
