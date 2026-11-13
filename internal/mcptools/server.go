package mcptools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/net/http/httpguts"
)

type Transport string

const (
	HTTP  Transport = "streamable-http"
	Stdio Transport = "stdio"
)

type Server struct {
	Name      string    `json:"name"`
	Transport Transport `json:"transport,omitempty"`
	URL       string    `json:"url,omitempty"`
	Command   string    `json:"command,omitempty"`
	Args      []string  `json:"args,omitempty"`
	Headers   []Header  `json:"headers,omitempty"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (s Server) httpHeaders() (http.Header, error) {
	headers := make(http.Header, len(s.Headers))
	for _, header := range s.Headers {
		name, value := http.CanonicalHeaderKey(header.Name), header.Value
		switch name {
		case "Host", "Content-Length", "Transfer-Encoding", "Connection", "Upgrade", "Te", "Trailer",
			"Keep-Alive", "Proxy-Connection", "Proxy-Authorization", "Content-Type", "Accept",
			"Mcp-Session-Id", "Mcp-Protocol-Version", "Last-Event-Id":
			return nil, errors.New(locales.ReservedMcpHeader())
		}
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) ||
			strings.TrimSpace(value) == "" || len(headers.Values(name)) != 0 {
			return nil, errors.New(locales.InvalidMcpHeader())
		}
		headers.Set(name, value)
	}
	return headers, nil
}

type mcpHeaderTransport struct {
	base    http.RoundTripper
	headers http.Header
	origin  *url.URL
}

func (t mcpHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != t.origin.Scheme || !strings.EqualFold(req.URL.Host, t.origin.Host) {
		return nil, errors.New("MCP request origin changed")
	}
	request := req.Clone(req.Context())
	for name, values := range t.headers {
		request.Header.Set(name, values[0])
	}
	return t.base.RoundTrip(request)
}

var ServerName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,20}$`)
var mcpToolName = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func (s Server) Kind() Transport {
	if s.Transport == "" {
		return HTTP // Existing saved servers only have a URL.
	}
	return s.Transport
}

func (t Transport) Label() string {
	if t == Stdio {
		return "Stdio"
	}
	return "Streamable HTTP"
}

func (s Server) Validate() error {
	switch s.Kind() {
	case HTTP:
		u, err := url.Parse(s.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
			u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(s.URL, "#") {
			return errors.New(locales.InvalidMcpURL())
		}
		_, err = s.httpHeaders()
		return err
	case Stdio:
		if s.Command == "" || strings.ContainsAny(s.Command, "\x00\r\n") ||
			(strings.ContainsRune(s.Command, filepath.Separator) && !filepath.IsAbs(s.Command)) {
			return errors.New(locales.InvalidMcpCommand())
		}
		if slices.ContainsFunc(s.Args, func(arg string) bool { return strings.ContainsRune(arg, 0) }) {
			return errors.New(locales.InvalidMcpArgs())
		}
	default:
		return fmt.Errorf("unsupported MCP transport: %q", s.Transport)
	}
	return nil
}

func (s Server) ConnectTransport(ctx context.Context, network http.RoundTripper) (mcp.Transport, localtools.MCPTarget, error) {
	if s.Kind() == Stdio {
		cmd := exec.CommandContext(ctx, s.Command, s.Args...)
		cmd.Stderr = os.Stderr
		args, _ := json.Marshal(s.Args)
		digest := sha256.Sum256(append([]byte(cmd.Path+"\x00"), args...))
		return &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second},
			localtools.MCPTarget{Label: cmd.Path, Version: fmt.Sprintf("stdio:%x", digest)}, nil
	}
	client := &http.Client{
		Transport:     network,
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	target := localtools.MCPTarget{Label: s.URL}
	if len(s.Headers) != 0 {
		headers, err := s.httpHeaders()
		if err != nil {
			return nil, target, err
		}
		origin, err := url.Parse(s.URL)
		if err != nil {
			return nil, target, err
		}
		client.Transport = mcpHeaderTransport{base: network, headers: headers, origin: origin}
		encoded, _ := json.Marshal(headers)
		target.Version = fmt.Sprintf("headers:%x", sha256.Sum256(encoded))
	}
	return &mcp.StreamableClientTransport{Endpoint: s.URL, HTTPClient: client}, target, nil
}

func ToolName(server, tool string) string {
	label := mcpToolName.ReplaceAllString(tool, "_")
	digest := sha256.Sum256([]byte(server + "\x00" + tool))
	return fmt.Sprintf("mcp_%s_%s_%x", server, label[:min(len(label), 25)], digest[:6])
}
