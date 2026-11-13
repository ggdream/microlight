package localtools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/pkg/diff"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type Mode string

const (
	Ask   Mode = "ask"
	Smart Mode = "smart"
	Full  Mode = "full"

	AllowOnce     = "允许一次"
	Deny          = "拒绝"
	Read          = "read_local_file"
	List          = "list_local_directory"
	Write         = "write_local_file"
	Fetch         = "fetch_web_page"
	maxLocalWrite = 16 << 10
	maxLocalRead  = 64 << 10
)

func (mode Mode) Normalized() Mode {
	switch mode {
	case Smart, Full:
		return mode
	default:
		return Ask
	}
}

func (mode Mode) Label() string {
	switch mode {
	case Smart:
		return locales.ModeSmart()
	case Full:
		return locales.ModeFull()
	default:
		return locales.ModeAsk()
	}
}

type sessionAccessKey struct{}

type Access struct {
	workspace string
	mode      atomic.Value
}

func (a *Access) SetMode(mode Mode) {
	a.mode.Store(mode.Normalized())
}

func NewAccess(workspace string, mode Mode) *Access {
	access := &Access{workspace: workspace}
	access.SetMode(mode)
	return access
}

func WithAccess(ctx context.Context, access *Access) context.Context {
	return context.WithValue(ctx, sessionAccessKey{}, access)
}

func Workspace(ctx context.Context, fallback string) string {
	if access, ok := ctx.Value(sessionAccessKey{}).(*Access); ok {
		return access.workspace
	}
	return fallback
}

type Approval struct {
	Target  string
	Preview string `json:",omitempty"`
}

type localOperation struct {
	webClient             *http.Client
	name, target, content string
	root, relative        string
	exists, sensitive     bool
	mcp                   bool
	fileVersion           string
	fileMode              os.FileMode
	callID                string
}

type localArguments struct {
	Path    string  `json:"path"`
	URL     string  `json:"url"`
	Content *string `json:"content"`
	Command string  `json:"command"`
}

type Middleware struct {
	*adk.BaseChatModelAgentMiddleware
	Workspace      string
	Mode           Mode
	WebClient      *http.Client
	MCPTargets     map[string]MCPTarget
	EncodeApproval func(question string, approval Approval) ([]byte, error)
}

func (h Middleware) ChatModelAgentMiddlewareName() string {
	return "microlight-local-permissions"
}

func (h Middleware) BeforeAgent(ctx context.Context, run *adk.ChatModelAgentContext[*schema.Message]) (context.Context, *adk.ChatModelAgentContext[*schema.Message], error) {
	workspace := Workspace(ctx, h.Workspace)
	if workspace == "" {
		return ctx, run, nil
	}
	updated := *run
	updated.Instruction += fmt.Sprintf("\nThe workspace path is %q.", workspace)
	return ctx, &updated, nil
}

type approvedOperationKey struct{}

func (h Middleware) WrapInvokableToolCall(_ context.Context, next adk.InvokableToolCallEndpoint, call *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	target, isMCP := h.MCPTargets[call.Name]
	switch call.Name {
	case Read, List, Write, Fetch, Execute:
	default:
		if !isMCP {
			return next, nil
		}
	}
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		wasInterrupted, hasState, saved := tool.GetInterruptState[string](ctx)
		targeted, hasAnswer, answer := tool.GetResumeContext[string](ctx)
		if targeted && hasAnswer && answer == Deny {
			return "用户拒绝了此操作。请勿尝试绕过或再次请求同一操作。", nil
		}
		policy := h
		access, scoped := ctx.Value(sessionAccessKey{}).(*Access)
		if scoped {
			policy.Workspace = access.workspace
		}
		op := localOperation{name: call.Name, target: target.Label, fileVersion: target.Version, content: args, mcp: isMCP}
		if !isMCP {
			var err error
			op, err = policy.resolve(call.Name, args)
			if err != nil {
				return "无法执行：" + err.Error(), nil
			}
		}
		// Bind the decision to this tool call and the exact resolved target and arguments.
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(call.CallID+"\x00"+op.target+"\x00"+op.fileVersion+"\x00"+args)))
		if wasInterrupted && (!hasState || saved != fingerprint) {
			return "操作目标已变化，请重新请求批准。", nil
		}
		if scoped {
			policy.Mode = access.mode.Load().(Mode)
		}
		if wasInterrupted || policy.needsApproval(op) {
			if !targeted || !hasAnswer {
				data, err := h.EncodeApproval(op.title(), Approval{Target: op.target, Preview: op.content})
				if err != nil {
					return "", err
				}
				return "", tool.StatefulInterrupt(ctx, string(data), fingerprint)
			}
			if !wasInterrupted || answer != AllowOnce {
				return "未取得该操作的有效批准。", nil
			}
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		op.webClient = h.WebClient
		op.callID = call.CallID
		return next(context.WithValue(ctx, approvedOperationKey{}, op), args, opts...)
	}, nil
}

func (h Middleware) needsApproval(op localOperation) bool {
	if op.name == Execute {
		return h.Mode.Normalized() != Full
	}
	switch h.Mode.Normalized() {
	case Full:
		return false
	case Smart:
		return op.root == "" || op.sensitive || (op.name == Write && op.exists)
	default:
		return op.root == "" || op.sensitive || op.name == Write
	}
}

func (op localOperation) title() string {
	if op.mcp {
		return locales.McpToolCall() + " · " + op.name
	}
	switch op.name {
	case Execute:
		return "运行命令"
	case Read:
		return "读取文件"
	case List:
		return "查看目录"
	case Write:
		if op.exists {
			return "覆盖文件"
		}
		return "创建文件"
	default:
		return "访问网页"
	}
}

func (h Middleware) resolve(name, raw string) (localOperation, error) {
	op := localOperation{name: name}
	var args localArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return op, errors.New("工具参数格式无效")
	}
	if name == Execute {
		if strings.TrimSpace(args.Command) == "" || len(args.Command) > maxLocalWrite || strings.ContainsRune(args.Command, 0) {
			return op, errors.New("需要有效的命令，且不能超过 16 KiB")
		}
		dir := h.Workspace
		if dir == "" {
			var err error
			dir, err = os.UserHomeDir()
			if err != nil {
				return op, err
			}
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return op, err
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() || !filepath.IsAbs(resolved) {
			return op, errors.New("命令的工作目录不可用")
		}
		op.target, op.content = resolved, args.Command
		return op, nil
	}
	if name == Fetch {
		u, err := url.Parse(strings.TrimSpace(args.URL))
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return op, errors.New("需要有效且不含登录凭据的 HTTP(S) 地址")
		}
		u.Fragment = ""
		op.target = u.String()
		return op, nil
	}
	path := args.Path
	if strings.TrimSpace(path) == "" {
		return op, errors.New("请指定路径")
	}
	if !filepath.IsAbs(path) {
		if h.Workspace == "" {
			return op, errors.New("未设置工作区，请使用绝对路径")
		}
		path = filepath.Join(h.Workspace, path)
	}
	path = filepath.Clean(path)
	var err error
	if name == Write {
		if args.Content == nil || len(*args.Content) > maxLocalWrite {
			return op, errors.New("待写文本必须明确提供，且不能超过 16 KiB")
		}
		op.content = *args.Content
		parent, resolveErr := filepath.EvalSymlinks(filepath.Dir(path))
		if resolveErr != nil {
			return op, resolveErr
		}
		op.target = filepath.Join(parent, filepath.Base(path))
		info, statErr := os.Lstat(op.target)
		switch {
		case statErr == nil:
			if !info.Mode().IsRegular() {
				return op, errors.New("仅支持写入普通文件")
			}
			op.exists = true
			op.fileMode = info.Mode().Perm()
			op.fileVersion = localFileVersion(info)
		case !errors.Is(statErr, os.ErrNotExist):
			return op, statErr
		}
	} else {
		op.target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return op, err
		}
	}
	if h.Workspace != "" {
		relative, err := filepath.Rel(h.Workspace, op.target)
		if err == nil && filepath.IsLocal(relative) {
			op.root, op.relative = h.Workspace, relative
			for _, part := range strings.Split(relative, string(filepath.Separator)) {
				part = strings.ToLower(part)
				if strings.HasPrefix(part, ".") && part != "." ||
					strings.Contains(part, "secret") || strings.Contains(part, "credential") ||
					strings.Contains(part, "password") || strings.HasSuffix(part, ".pem") ||
					strings.HasSuffix(part, ".key") {
					op.sensitive = true
				}
			}
		}
	}
	return op, nil
}

type Tool string

func (t Tool) Info(context.Context) (*schema.ToolInfo, error) {
	path := &schema.ParameterInfo{Type: schema.String, Required: true, Desc: "Absolute path, or path relative to the configured workspace."}
	params := map[string]*schema.ParameterInfo{"path": path}
	var desc string
	switch string(t) {
	case Read:
		desc = "Read a local UTF-8 text file, up to 64 KiB. Use only for files relevant to the user's request."
	case List:
		desc = "List a local directory, at most 200 entry names."
	case Write:
		desc = "Create or replace a local UTF-8 text file with the complete content, up to 16 KiB. Parent directory must exist."
		params["content"] = &schema.ParameterInfo{Type: schema.String, Required: true, Desc: "Complete UTF-8 text to write."}
	case Fetch:
		desc = "Fetch the text of an HTTP(S) URL with GET. Redirects are not followed."
		params = map[string]*schema.ParameterInfo{"url": {Type: schema.String, Required: true, Desc: "The exact URL to fetch."}}
	}
	return &schema.ToolInfo{Name: string(t), Desc: desc, ParamsOneOf: schema.NewParamsOneOfByParams(params)}, nil
}

func (t Tool) InvokableRun(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
	op, ok := ctx.Value(approvedOperationKey{}).(localOperation)
	if !ok || op.name != string(t) {
		return "操作未经权限检查。", nil
	}
	result, diff, err := op.execute(ctx)
	if err != nil {
		return "无法执行：" + err.Error(), nil
	}
	if report, ok := ctx.Value(reportToolResultKey{}).(func(Result)); ok && op.callID != "" {
		preview := result
		if op.name == Write {
			preview = op.content
		}
		report(Result{CallID: op.callID, Target: op.target, Text: preview, Diff: diff})
	}
	return result, nil
}

func (op localOperation) execute(ctx context.Context) (string, *diff.Lines, error) {
	if op.name == Fetch {
		text, err := fetchLocalAgentURL(ctx, op.webClient, op.target)
		return text, nil, err
	}
	base, path := op.root, op.relative
	if base == "" {
		base, path = filepath.Dir(op.target), filepath.Base(op.target)
		if base == op.target {
			path = "."
		}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	if op.name == Write {
		return op.write(root, path)
	}
	info, err := root.Stat(path)
	if err != nil {
		return "", nil, err
	}
	if (op.name == Read && !info.Mode().IsRegular()) || (op.name == List && !info.IsDir()) {
		return "", nil, errors.New("目标类型与操作不匹配")
	}
	f, err := root.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	if op.name == List {
		names, err := f.Readdirnames(200)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", nil, err
		}
		data, err := json.Marshal(names)
		return string(data), nil, err
	}
	text, err := readLocalAgentText(f)
	return text, nil, err
}

func (op localOperation) write(root *os.Root, path string) (string, *diff.Lines, error) {
	temp := filepath.Join(filepath.Dir(path), ".microlight-"+uuid.NewV7().String())
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, err
	}
	defer root.Remove(temp)
	before, canDiff := "", true
	if op.exists {
		before, canDiff = op.readPreviousText(root, path)
	}
	if op.exists {
		if err := file.Chmod(op.fileMode); err != nil {
			file.Close()
			return "", nil, err
		}
	}
	_, writeErr := io.WriteString(file, op.content)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", nil, err
	}
	if op.exists {
		info, statErr := root.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || localFileVersion(info) != op.fileVersion {
			return "", nil, errors.New("文件已变化，请重新请求批准")
		}
		err = root.Rename(temp, path)
	} else {
		// Atomically create the target only if it is still absent.
		err = root.Link(temp, path)
	}
	if err != nil {
		return "", nil, err
	}
	var changes *diff.Lines
	if canDiff {
		changes = diff.Compare(before, op.content)
	}
	return "已写入 " + op.target, changes, nil
}

func (op localOperation) readPreviousText(root *os.Root, path string) (string, bool) {
	if info, err := root.Lstat(path); err != nil || !info.Mode().IsRegular() || localFileVersion(info) != op.fileVersion {
		return "", false
	}
	file, err := root.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() || localFileVersion(info) != op.fileVersion {
		return "", false
	}
	content, err := readLocalAgentText(file)
	return content, err == nil
}

func localFileVersion(info os.FileInfo) string {
	return fmt.Sprintf("%d:%d:%d", info.Size(), info.ModTime().UnixNano(), info.Mode().Perm())
}

var localWebClient = &http.Client{
	Transport:     &http.Transport{IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4},
	Timeout:       15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func fetchLocalAgentURL(ctx context.Context, client *http.Client, target string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	if client == nil {
		client = localWebClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return readLocalAgentText(response.Body)
}

func readLocalAgentText(reader io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxLocalRead+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxLocalRead {
		return "", errors.New("目标内容超过 64 KiB")
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return "", errors.New("目标不是 UTF-8 文本")
	}
	return string(data), nil
}

// ReadPreview resolves and reads a file or directory requested by the user.
func ReadPreview(ctx context.Context, workspace, kind, path string) (string, error) {
	if kind != Read && kind != List {
		return "", errors.New("unsupported preview tool")
	}
	args, err := json.Marshal(localArguments{Path: path})
	if err != nil {
		return "", err
	}
	op, err := (Middleware{Workspace: workspace}).resolve(kind, string(args))
	if err != nil {
		return "", err
	}
	content, _, err := op.execute(ctx)
	return content, err
}
