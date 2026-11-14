package workspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
)

type e2bTestTransport func(*http.Request) (*http.Response, error)

func (f e2bTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func connectFrame(flag byte, payload string) []byte {
	frame := make([]byte, 5, 5+len(payload))
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
	return append(frame, payload...)
}

func TestE2BCommandTransport(t *testing.T) {
	var calls int
	client := &http.Client{Transport: e2bTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		var data []byte
		switch req.URL.String() {
		case "https://api.e2b.app/sandboxes/test-vm":
			if req.Method != http.MethodGet || req.Header.Get("X-API-Key") != "account-key" || req.Header.Get("X-Access-Token") != "" {
				return nil, fmt.Errorf("unexpected sandbox lookup")
			}
			data = []byte(`{"sandboxID":"test-vm","state":"running","domain":"e2b.app","envdAccessToken":"sandbox-key"}`)
		case "https://49983-test-vm.e2b.app/process.Process/Start":
			if req.Method != http.MethodPost || req.Header.Get("X-Access-Token") != "sandbox-key" ||
				req.Header.Get("X-API-Key") != "" || req.Header.Get("Content-Type") != "application/connect+json" {
				return nil, fmt.Errorf("unexpected process start headers")
			}
			defer req.Body.Close()
			body, err := io.ReadAll(req.Body)
			if err != nil || len(body) < 5 || body[0] != 0 || int(binary.BigEndian.Uint32(body[1:5])) != len(body)-5 {
				return nil, fmt.Errorf("unexpected process start envelope")
			}
			var start struct {
				Process struct {
					Cmd, Cwd string
					Args     []string
				}
				Stdin bool
			}
			if err := json.Unmarshal(body[5:], &start); err != nil || start.Process.Cmd != "/bin/sh" ||
				start.Process.Cwd != "/home/user" || len(start.Process.Args) != 2 || start.Process.Args[0] != "-c" || start.Stdin {
				return nil, fmt.Errorf("invalid process I/O: %s", body[5:])
			}
			switch calls {
			case 2:
				if !strings.Contains(start.Process.Args[1], "flock") {
					t.Error("missing recovery capability probe")
				}
			case 3:
				if !strings.Contains(start.Process.Args[1], "printf test") {
					t.Error("original command absent from launch")
				}
			case 4, 5:
				data = connectFrame(0, `{"event":{"data":{"stdout":"ZXhpdCAwCnRlc3Q="}}}`)
			}
			data = append(data, connectFrame(0, `{"event":{"end":{"exited":true}}}`)...)
			data = append(data, connectFrame(2, `{}`)...)
		default:
			return nil, fmt.Errorf("unexpected destination: %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)),
			Header: http.Header{"Content-Type": {"application/connect+json"}}}, nil
	})}
	rt, err := openE2B(t.Context(), Target{Kind: "e2b", Resource: "test-vm", Directory: "/home/user"}, "account-key", client)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	ops, err := DiscoverOperations(t.Context(), rt)
	if err != nil || ops.Commands == nil || !ops.Capabilities.RecoverableShell || ops.Capabilities.Files {
		t.Fatalf("capabilities = %+v, %v", ops.Capabilities, err)
	}
	run, err := ops.Commands.StartCommand(context.Background(), &backgroundshell.StartCommandRequest{TaskID: "task-1", Command: "printf test"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(t.Context())
	if err != nil || !strings.Contains(string(result.Data), `"Output":"test"`) || calls != 5 {
		t.Fatalf("command result = %+v, %v, calls %d", result, err, calls)
	}
}

func TestE2BProcessRequiresTerminalEnvelope(t *testing.T) {
	ended := connectFrame(0, `{"event":{"end":{"exited":true}}}`)
	if _, err := readE2BProcess(bytes.NewReader(ended)); err == nil {
		t.Fatal("accepted truncated stream")
	}
	if _, err := readE2BProcess(bytes.NewReader(append(ended, connectFrame(2, `{"error":{"code":"unavailable"}}`)...))); err == nil {
		t.Fatal("accepted stream error")
	}
}
