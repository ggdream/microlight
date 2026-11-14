package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundshell "github.com/cloudwego/eino/adk/backgroundtask/shell"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/cloudwego/eino/adk/filesystem"
	_ "modernc.org/sqlite"
)

type desktopCommandRequest struct {
	Directory, ID, Command string
	TimeoutSeconds         int `json:",omitempty"`
}

const desktopCommandMissing = "command-task-not-found"

type desktopCommandStore struct {
	db     *sql.DB
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func openDesktopCommandStore(root string) (*desktopCommandStore, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "microlight", "desktop-commands")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	name := filepath.Join(dir, fmt.Sprintf("%x.sqlite3", sha256.Sum256([]byte(root))))
	return newDesktopCommandStore(name)
}

func newDesktopCommandStore(name string) (*desktopCommandStore, error) {
	db, err := sql.Open("sqlite", name)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=3000;
		CREATE TABLE IF NOT EXISTS commands (
			id TEXT PRIMARY KEY, signature TEXT NOT NULL, outcome BLOB
		)`); err != nil {
		db.Close()
		return nil, err
	}
	return &desktopCommandStore{db: db, active: make(map[string]context.CancelFunc)}, nil
}

func (s *desktopCommandStore) serve(ctx context.Context, w http.ResponseWriter, r *http.Request, files *desktopFiles, method string) {
	if method != "start" && method != "stop" && method != "status" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req desktopCommandRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid command arguments", http.StatusBadRequest)
		return
	}
	if req.ID == "" || len(req.ID) > 256 || strings.TrimSpace(req.Command) == "" ||
		len(req.Command) > 16<<10 || strings.ContainsRune(req.Command, 0) {
		http.Error(w, "invalid task ID or command", http.StatusBadRequest)
		return
	}
	name, err := desktopRelativePath(req.Directory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if desktopEnvironment().OS == "windows" {
		name = strings.ToLower(name)
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(name+"\x00"+req.ID)))
	signature := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Command)))
	seconds := req.TimeoutSeconds
	if seconds <= 0 {
		seconds = maxCommandSeconds
	}
	timeout := time.Duration(min(seconds, maxCommandSeconds)) * time.Second
	outcome, err := s.command(ctx, method, id, signature, req.Command, timeout, func() (string, error) {
		return files.commandDirectory(ctx, req.Directory)
	})
	switch {
	case errors.Is(err, backgroundtask.ErrNotFound):
		http.Error(w, desktopCommandMissing, http.StatusNotFound)
	case err != nil:
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(outcome)
	}
}

func (s *desktopCommandStore) command(ctx context.Context, method, id, signature, command string, timeout time.Duration, directory func() (string, error)) (*backgroundtool.Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var savedSignature string
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT signature, outcome FROM commands WHERE id=?", id).Scan(&savedSignature, &data)
	switch {
	case err == nil:
		if savedSignature != signature {
			return nil, errors.New("task ID is already bound to another command")
		}
		if data != nil {
			var outcome backgroundtool.Outcome
			err := json.Unmarshal(data, &outcome)
			return &outcome, err
		}
		cancel, running := s.active[id]
		if !running {
			return nil, errors.New("desktop command outcome is unknown")
		}
		if method == "stop" {
			cancel()
		}
		return nil, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	switch method {
	case "status":
		return nil, backgroundtask.ErrNotFound
	case "stop":
		outcome := &backgroundtool.Outcome{Status: backgroundtask.StatusCanceled}
		data, err := json.Marshal(outcome)
		if err != nil {
			return nil, err
		}
		_, err = s.db.ExecContext(ctx, "INSERT INTO commands VALUES (?, ?, ?)", id, signature, data)
		return outcome, err
	case "start":
		if len(s.active) >= 8 {
			return nil, errors.New("desktop command limit reached")
		}
		dir, err := directory()
		if err != nil {
			return nil, err
		}
		// Commit the claim before launching. After a process crash, an
		// unresolved claim stays unknown and can never start a second time.
		if _, err := s.db.ExecContext(ctx, "INSERT INTO commands VALUES (?, ?, NULL)", id, signature); err != nil {
			return nil, err
		}
		runCtx, cancel := context.WithTimeout(context.Background(), timeout)
		s.active[id] = cancel
		go s.run(runCtx, cancel, id, dir, command)
		return nil, nil
	default:
		return nil, errors.New("unknown command operation")
	}
}

func (s *desktopCommandStore) run(ctx context.Context, cancel context.CancelFunc, id, dir, command string) {
	defer cancel()
	outcome := runDesktopCommand(ctx, dir, command)
	s.mu.Lock()
	defer s.mu.Unlock()
	defer delete(s.active, id)
	data, err := json.Marshal(outcome)
	if err == nil {
		// A failed save leaves the launch claim in place.
		_, _ = s.db.Exec("UPDATE commands SET outcome=? WHERE id=?", data, id)
	}
}

func runDesktopCommand(ctx context.Context, dir, command string) *backgroundtool.Outcome {
	cmd := desktopShellCommand(command)
	cmd.Dir = dir
	env := cmd.Environ()
	cmd.Env = make([]string, 0, len(env))
	for _, value := range env {
		name, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(name, "MICROLIGHT_DESKTOP_TOKEN") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	out := &boundedOutput{limit: fileLimit}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = time.Second
	err := runDesktopProcess(ctx, cmd)
	if errors.Is(err, context.Canceled) {
		return &backgroundtool.Outcome{Status: backgroundtask.StatusCanceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code := 124
		response := filesystem.ExecuteResponse{ExitCode: &code, TimedOut: true, Output: strings.ToValidUTF8(out.data.String(), "�"), Truncated: out.truncated}
		data, _ := json.Marshal(response)
		return &backgroundtool.Outcome{Status: backgroundtask.StatusCompleted, Data: data}
	}
	response, err := shellResult(out.data.String(), out.truncated, err)
	if err != nil {
		return &backgroundtool.Outcome{Status: backgroundtask.StatusFailed, Error: err.Error()}
	}
	data, _ := json.Marshal(response)
	return &backgroundtool.Outcome{Status: backgroundtask.StatusCompleted, Data: data}
}

func waitDesktopProcess(ctx context.Context, cmd *exec.Cmd, terminate func()) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		terminate()
		if errors.Is(err, exec.ErrWaitDelay) {
			return nil
		}
		return err
	case <-ctx.Done():
		terminate()
		<-done
		return ctx.Err()
	}
}

type desktopCommandClient struct{ desktop *remoteDesktop }

func (d *remoteDesktop) Commands() backgroundshell.RecoverableShell {
	if d.directory == "" || !d.info.Commands {
		return nil
	}
	return desktopCommandClient{d}
}

func (c desktopCommandClient) StartCommand(ctx context.Context, req *backgroundshell.StartCommandRequest) (backgroundtool.Run, error) {
	return c.StartCommandOnce(ctx, req)
}

func (c desktopCommandClient) StartCommandOnce(ctx context.Context, req *backgroundshell.StartCommandRequest) (backgroundtool.Run, error) {
	if req == nil {
		return nil, errors.New("command request is required")
	}
	return c.open(ctx, "start", req.TaskID, req.Command)
}

func (c desktopCommandClient) RecoverCommand(ctx context.Context, req *backgroundshell.RecoverCommandRequest) (backgroundtool.Run, error) {
	if req == nil {
		return nil, errors.New("command request is required")
	}
	return c.open(ctx, "status", req.TaskID, req.Command)
}

func (c desktopCommandClient) StopCommand(ctx context.Context, req *backgroundshell.RecoverCommandRequest) (backgroundtool.Run, error) {
	if req == nil {
		return nil, errors.New("command request is required")
	}
	return c.open(ctx, "stop", req.TaskID, req.Command)
}

func (c desktopCommandClient) open(ctx context.Context, method, id, command string) (backgroundtool.Run, error) {
	run := &desktopCommandRun{desktop: c.desktop, req: desktopCommandRequest{Directory: c.desktop.directory, ID: id, Command: command}}
	outcome, err := run.call(ctx, method)
	if err != nil {
		return nil, err
	}
	run.outcome = outcome
	return run, nil
}

type desktopCommandRun struct {
	desktop *remoteDesktop
	req     desktopCommandRequest
	outcome *backgroundtool.Outcome
}

func (r *desktopCommandRun) call(ctx context.Context, method string) (*backgroundtool.Outcome, error) {
	var outcome *backgroundtool.Outcome
	req := r.req
	if method == "start" && commandSeconds(ctx) != maxCommandSeconds {
		req.TimeoutSeconds = commandSeconds(ctx)
	}
	err := r.desktop.call(ctx, "commands/"+method, req, &outcome)
	var status *desktopHTTPError
	switch {
	case err == nil:
		return outcome, nil
	case errors.As(err, &status) && status.code == http.StatusNotFound && status.detail == desktopCommandMissing:
		return nil, backgroundtask.ErrNotFound
	case status != nil && status.code == http.StatusUnprocessableEntity:
		return nil, err
	default:
		return nil, &CommandTransportError{err}
	}
}

func (r *desktopCommandRun) Wait(ctx context.Context) (*backgroundtool.Outcome, error) {
	if r.outcome != nil {
		return r.outcome, ctx.Err()
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			outcome, err := r.call(ctx, "status")
			if outcome != nil || err != nil {
				return outcome, err
			}
		}
	}
}

func (r *desktopCommandRun) Stop(ctx context.Context) error {
	_, err := r.call(ctx, "stop")
	return err
}
