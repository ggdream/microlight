package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	"github.com/ggdream/microlight/pkg/proxy"
	"github.com/zalando/go-keyring"
)

var e2bSandboxID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,56}$`)
var e2bSandboxDomain = regexp.MustCompile(`^([a-z0-9-]+\.)*e2b\.(app|dev)$`)

type e2bRuntime struct {
	posixFiles
	client          *http.Client
	target          Target
	endpoint, token string
	files, jobs     bool
}

func connectE2B(ctx context.Context, target Target, key string) (Runtime, error) {
	config, _ := ctx.Value(proxyConfigKey{}).(proxy.Config)
	transport, err := proxy.NewTransport(config)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	rt, err := openE2B(ctx, target, key, client)
	if err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	return rt, nil
}

func openE2B(ctx context.Context, target Target, key string, client *http.Client) (*e2bRuntime, error) {
	if !e2bSandboxID.MatchString(target.Resource) || !path.IsAbs(target.Directory) || strings.ContainsRune(target.Directory, 0) {
		return nil, errors.New("enter an E2B sandbox ID and an absolute working directory")
	}
	if key == "" && target.Credential != "" {
		var err error
		key, err = keyring.Get(keyringService, target.Credential)
		if err != nil {
			return nil, err
		}
	}
	if key == "" {
		return nil, errors.New("an E2B API key is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.e2b.app/sandboxes/"+target.Resource, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("E2B sandbox lookup returned HTTP %d", resp.StatusCode)
	}
	var info struct {
		SandboxID, State, Domain, EnvdAccessToken string
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return nil, err
	}
	if info.SandboxID != target.Resource || info.State != "running" || info.EnvdAccessToken == "" {
		return nil, errors.New("the E2B sandbox must be running with envd access enabled")
	}
	domain := info.Domain
	if domain == "" {
		domain = "e2b.app"
	}
	if !e2bSandboxDomain.MatchString(domain) {
		return nil, errors.New("unexpected E2B sandbox domain")
	}
	rt := &e2bRuntime{client: client, target: target, endpoint: "https://49983-" + target.Resource + "." + domain, token: info.EnvdAccessToken}
	rt.posixFiles = posixFiles{directory: target.Directory, command: rt.fileCommand}
	_, fileErr := rt.control(ctx, posixFileProbe+"; command -v base64 >/dev/null")
	rt.files = fileErr == nil
	rt.jobs = supportsRemoteCommands(ctx, rt.control)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !rt.files && !rt.jobs {
		return nil, fmt.Errorf("could not verify file or recoverable command support in the E2B sandbox: %w", fileErr)
	}
	return rt, nil
}

func (*e2bRuntime) Environment() Environment { return Environment{OS: "linux", Shell: "/bin/sh"} }
func (r *e2bRuntime) Close() error           { r.client.CloseIdleConnections(); return nil }
func (r *e2bRuntime) GetCapabilities(ctx context.Context) (Capabilities, error) {
	return Capabilities{Files: r.files, Shell: r.jobs, RecoverableShell: r.jobs}, ctx.Err()
}

func (r *e2bRuntime) Commands() backgroundshell.RecoverableShell {
	if !r.jobs {
		return nil
	}
	return remoteCommands{run: r.control, directory: r.target.Directory, scope: r.target.Identity()}
}

func (r *e2bRuntime) fileCommand(ctx context.Context, script, input string) (string, error) {
	if input != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(input))
		// Decode the input before invoking the file operation so a decode
		// failure cannot turn a write into an empty replacement.
		script = "input=$(mktemp) || exit; trap 'rm -f -- \"$input\"' EXIT; " +
			"printf '%s' " + quotePOSIX(encoded) + " | base64 -d > \"$input\" || exit; " +
			"( " + script + " ) < \"$input\""
	}
	return r.control(ctx, script)
}

func (r *e2bRuntime) control(ctx context.Context, script string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"process": map[string]any{"cmd": "/bin/sh", "args": []string{"-c", script}, "cwd": r.target.Directory},
		"stdin":   false,
	})
	if err != nil {
		return "", err
	}
	frame := make([]byte, 5, len(body)+5)
	binary.BigEndian.PutUint32(frame[1:], uint32(len(body)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/process.Process/Start", bytes.NewReader(append(frame, body...)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Connect-Accept-Encoding", "identity")
	req.Header.Set("X-Access-Token", r.token)
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("E2B process service returned HTTP %d", resp.StatusCode)
	}
	if contentType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";"); contentType != "application/connect+json" {
		return "", errors.New("unexpected E2B process response format")
	}
	return readE2BProcess(resp.Body)
}

func readE2BProcess(reader io.Reader) (string, error) {
	out := &boundedOutput{limit: remoteResponseLimit}
	var exited bool
	for {
		var header [5]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return "", err
		}
		size := binary.BigEndian.Uint32(header[1:])
		if size > 1<<20 {
			return "", errors.New("E2B process response frame too large")
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return "", err
		}
		switch header[0] {
		case 2:
			var end struct{ Error json.RawMessage }
			if err := json.Unmarshal(payload, &end); err != nil {
				return "", err
			}
			if len(end.Error) != 0 || !exited || out.truncated {
				return "", errors.New("E2B command control did not complete successfully")
			}
			return out.data.String(), nil
		case 0:
			var message struct {
				Event struct {
					Data struct{ Stdout []byte }
					End  *struct {
						ExitCode int
						Exited   bool
						Error    string
					}
				}
			}
			if err := json.Unmarshal(payload, &message); err != nil {
				return "", err
			}
			_, _ = out.Write(message.Event.Data.Stdout)
			if end := message.Event.End; end != nil {
				if !end.Exited || end.ExitCode != 0 || end.Error != "" {
					return "", errors.New("E2B command control failed")
				}
				exited = true
			}
		default:
			return "", errors.New("unsupported E2B response frame")
		}
	}
}
