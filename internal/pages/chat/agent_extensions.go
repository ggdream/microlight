package chat

import (
	"context"
	"net/http"
	"time"

	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	skillset "github.com/ggdream/microlight/internal/skills"

	"github.com/cloudwego/eino-ext/components/tool/mcp/officialmcp"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type agentExtensions struct {
	tools    []tool.BaseTool
	targets  map[string]localtools.MCPTarget
	skills   adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]
	sessions []*mcp.ClientSession
}

func (ext *agentExtensions) close() {
	for _, session := range ext.sessions {
		_ = session.Close()
	}
}

type extensionError struct {
	server string
	cause  error
}

func (e *extensionError) Error() string { return e.cause.Error() }
func (e *extensionError) Unwrap() error { return e.cause }

func (ext *agentExtensions) load(ctx context.Context, config modelSettings, skills *skillset.Catalog, network http.RoundTripper) error {
	if len(skills.Entries) != 0 {
		var err error
		ext.skills, err = skill.NewTyped(ctx, &skill.TypedConfig[*schema.AgenticMessage]{Backend: skills})
		if err != nil {
			return &extensionError{cause: err}
		}
	}
	ext.targets = make(map[string]localtools.MCPTarget)
	for _, server := range config.MCPServers {
		if err := ext.loadMCP(ctx, server, network); err != nil {
			return &extensionError{server: server.Name, cause: err}
		}
	}
	return nil
}

func (ext *agentExtensions) loadMCP(ctx context.Context, server mcptools.Server, network http.RoundTripper) error {
	transport, target, err := server.ConnectTransport(ctx, network)
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "microlight-chat", Version: "0.1.0"}, nil)
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return err
	}
	ext.sessions = append(ext.sessions, session)
	tools, err := officialmcp.GetTools(connectCtx, &officialmcp.Config{
		Cli:        session,
		ServerName: server.Name,
		ToolNameMapper: func(_ context.Context, info officialmcp.ToolNameMapperInput) (officialmcp.ToolNameMapperOutput, error) {
			name := mcptools.ToolName(server.Name, info.Tool.Name)
			ext.targets[name] = target
			return officialmcp.ToolNameMapperOutput{ExposedName: name}, nil
		},
	})
	if err != nil {
		return err
	}
	ext.tools = append(ext.tools, tools...)
	return nil
}
