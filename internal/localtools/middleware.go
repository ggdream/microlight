package localtools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unicode/utf8"
	"uuid"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/ggdream/microlight/assets/locales"
	project "github.com/ggdream/microlight/internal/workspace"
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

func WithRuntime(ctx context.Context, access *Access, runtime project.Runtime, target project.Target, operations project.Operations) context.Context {
	if access != nil {
		ctx = WithAccess(ctx, access)
	}
	return context.WithValue(ctx, runtimeAccessKey{}, runtimeAccess{runtime, target, operations})
}

type runtimeAccessKey struct{}
type runtimeAccess struct {
	runtime    project.Runtime
	target     project.Target
	operations project.Operations
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
	if selected, ok := ctx.Value(runtimeAccessKey{}).(runtimeAccess); ok && selected.target.Kind != "" {
		return ""
	}
	if access, ok := ctx.Value(sessionAccessKey{}).(*Access); ok {
		return access.workspace
	}
	return fallback
}

type Approval struct {
	Target    string
	Preview   string `json:",omitempty"`
	Automatic bool   `json:",omitempty"`
	CallID    string `json:",omitempty"`
}

type localOperation struct {
	runtime               project.Runtime
	operations            project.Operations
	webClient             *http.Client
	name, target, content string
	root, relative        string
	exists, sensitive     bool
	mcp                   bool
	approvalTitle         string
	fileVersion           string
	fileMode              os.FileMode
	callID                string
	webOffset             int
}

type localArguments struct {
	Path    string  `json:"path"`
	URL     string  `json:"url"`
	Offset  int     `json:"offset"`
	Content *string `json:"content"`
	Command string  `json:"command"`
}

type Middleware struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
	Workspace      string
	Mode           Mode
	WebClient      *http.Client
	MCPTargets     map[string]MCPTarget
	DriverTargets  map[string]DriverTarget
	EncodeApproval func(question string, approval Approval) ([]byte, error)
}

func (h Middleware) ChatModelAgentMiddlewareName() string {
	return "microlight-local-permissions"
}

func (h Middleware) BeforeAgent(ctx context.Context, run *adk.ChatModelAgentContext[*schema.AgenticMessage]) (context.Context, *adk.ChatModelAgentContext[*schema.AgenticMessage], error) {
	if remote, ok := ctx.Value(runtimeAccessKey{}).(runtimeAccess); ok && remote.target.Kind != "" {
		updated := *run
		env := remote.runtime.Environment()
		updated.Instruction += fmt.Sprintf("\nAvailable file, shell, browser and desktop tools operate in the selected %s execution environment (directory %q, OS %q, architecture %q, shell %q). Web fetch and configured MCP are managed by Microlight.", remote.target.Kind, remote.target.Directory, env.OS, env.Arch, env.Shell)
		return ctx, &updated, nil
	}
	workspace := Workspace(ctx, h.Workspace)
	if workspace == "" {
		return ctx, run, nil
	}
	updated := *run
	updated.Instruction += fmt.Sprintf("\nThe workspace path is %q.", workspace)
	return ctx, &updated, nil
}

type approvedOperationKey struct{}

// EnvironmentTools runs after tool registration so the model sees only the
// operations supported by its connected environment.
type EnvironmentTools struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
}

func (EnvironmentTools) ChatModelAgentMiddlewareName() string {
	return "microlight-environment-tools"
}

func (EnvironmentTools) BeforeAgent(ctx context.Context, run *adk.ChatModelAgentContext[*schema.AgenticMessage]) (context.Context, *adk.ChatModelAgentContext[*schema.AgenticMessage], error) {
	selected, ok := ctx.Value(runtimeAccessKey{}).(runtimeAccess)
	if !ok || (selected.operations.Capabilities.Files && selected.operations.Capabilities.Shell) {
		return ctx, run, nil
	}
	updated := *run
	updated.Tools = make([]tool.BaseTool, 0, len(run.Tools))
	for _, candidate := range run.Tools {
		info, err := candidate.Info(ctx)
		if err != nil {
			return ctx, run, err
		}
		switch info.Name {
		case Read, Write, List:
			if !selected.operations.Capabilities.Files {
				continue
			}
		case Execute:
			if !selected.operations.Capabilities.Shell {
				continue
			}
		}
		updated.Tools = append(updated.Tools, candidate)
	}
	return ctx, &updated, nil
}

func (h Middleware) WrapInvokableToolCall(_ context.Context, next adk.InvokableToolCallEndpoint, call *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	target, isMCP := h.MCPTargets[call.Name]
	driver, isDriver := h.DriverTargets[call.Name]
	switch call.Name {
	case Read, List, Write, Fetch, Execute:
	default:
		if !isMCP && !isDriver {
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
		if isDriver {
			op.target, op.fileVersion, op.approvalTitle = driver.Label, driver.Identity, driver.Title
		} else if !isMCP {
			var err error
			selected, connected := ctx.Value(runtimeAccessKey{}).(runtimeAccess)
			if connected && selected.target.Kind != "" && call.Name != Fetch {
				op, err = resolveRuntime(ctx, selected, call.Name, args)
			} else {
				op, err = policy.resolve(call.Name, args)
			}
			if err != nil {
				return "无法执行：" + err.Error(), nil
			}
			if connected && selected.target.Kind == "" && call.Name != Fetch {
				op.runtime, op.operations = selected.runtime, selected.operations
			}
		}
		// Bind the decision to this tool call and the exact resolved target and arguments.
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(call.CallID+"\x00"+op.target+"\x00"+op.fileVersion+"\x00"+args)))
		automaticState := fingerprint + "/automatic"
		if wasInterrupted && (!hasState || (saved != fingerprint && saved != automaticState)) {
			return "操作目标已变化，请重新请求批准。", nil
		}
		if scoped {
			policy.Mode = access.mode.Load().(Mode)
		}
		needsApproval := policy.needsApproval(op)
		journal, _ := ctx.Value(commandJournalKey{}).(CommandJournal)
		recoverableCommand := op.name == Execute && op.operations.Commands != nil && journal.Record != nil
		automatic := !wasInterrupted && !needsApproval && recoverableCommand
		if wasInterrupted || needsApproval || automatic {
			if !targeted || !hasAnswer {
				approval := Approval{Target: op.target, Preview: op.content, Automatic: automatic || (saved == automaticState && !needsApproval)}
				if recoverableCommand {
					approval.CallID = call.CallID
				}
				data, err := h.EncodeApproval(op.title(), approval)
				if err != nil {
					return "", err
				}
				if approval.Automatic {
					fingerprint = automaticState
				}
				return "", tool.StatefulInterrupt(ctx, string(data), fingerprint)
			}
			if !wasInterrupted || (answer != AllowOnce && (answer != ContinueCommand || saved != automaticState || needsApproval)) {
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

func (h Middleware) WrapEnhancedInvokableToolCall(ctx context.Context, next adk.EnhancedInvokableToolCallEndpoint, call *adk.ToolContext) (adk.EnhancedInvokableToolCallEndpoint, error) {
	if _, ok := h.DriverTargets[call.Name]; !ok {
		return next, nil
	}
	return func(ctx context.Context, args *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		var result *schema.ToolResult
		permission, err := h.WrapInvokableToolCall(ctx, func(ctx context.Context, raw string, opts ...tool.Option) (string, error) {
			var err error
			result, err = next(ctx, &schema.ToolArgument{Text: raw}, opts...)
			return "", err
		}, call)
		if err != nil {
			return nil, err
		}
		text, err := permission(ctx, args.Text, opts...)
		if err != nil || result != nil {
			return result, err
		}
		return &schema.ToolResult{Parts: []schema.ToolOutputPart{{Type: schema.ToolPartTypeText, Text: text}}}, nil
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
	if op.approvalTitle != "" {
		return op.approvalTitle
	}
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
		resolved, err := localCommandDirectory(h.Workspace)
		if err != nil {
			return op, err
		}
		op.target, op.content = resolved, args.Command
		return op, nil
	}
	if name == Fetch {
		u, err := webURL(strings.TrimSpace(args.URL))
		if err != nil {
			return op, err
		}
		if args.Offset < 0 {
			return op, errors.New("网页正文偏移量无效")
		}
		op.target, op.webOffset = u.String(), args.Offset
		return op, nil
	}
	return h.resolveLocalFile(name, args)
}

func (h Middleware) resolveLocalFile(name string, args localArguments) (localOperation, error) {
	op := localOperation{name: name}
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
		desc = "Read an HTTP(S) page as clean text with its title and final URL. Follows same-site redirects and reports other destinations. Returns up to 20,000 characters of content per chunk. Use next_offset to read more."
		params = map[string]*schema.ParameterInfo{
			"url":    {Type: schema.String, Required: true, Desc: "The URL to read."},
			"offset": {Type: schema.Integer, Desc: "Start at this UTF-8 byte offset in the extracted text. Omit for the first chunk, or use next_offset from the previous result."},
		}
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
	if local, ok := op.operations.Files.(LocalRuntime); ok {
		return local.executeApprovedFile(ctx, op)
	}
	if op.runtime != nil {
		return op.executeRuntime(ctx)
	}
	if op.name == Fetch {
		text, err := fetchLocalAgentURL(ctx, op.webClient, op.target, op.webOffset)
		return text, nil, err
	}
	local := localFilesystem{operation: op}
	return executeFiles(ctx, local, op)
}

type localFilesystem struct{ operation localOperation }

func (local localFilesystem) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	op := local.operation
	op.name, op.target = Read, req.FilePath
	text, _, err := op.executeLocalFile(ctx)
	return &filesystem.FileContent{Content: text}, err
}

func (local localFilesystem) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	op := local.operation
	op.name, op.target = List, req.Path
	text, _, err := op.executeLocalFile(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal([]byte(text), &names); err != nil {
		return nil, err
	}
	files := make([]filesystem.FileInfo, 0, len(names))
	for _, name := range names {
		files = append(files, filesystem.FileInfo{Path: name})
	}
	return files, nil
}

func (local localFilesystem) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	_, _, err := local.write(ctx, req)
	return err
}

func (local localFilesystem) write(ctx context.Context, req *filesystem.WriteRequest) (string, *diff.Lines, error) {
	op := local.operation
	op.name, op.target, op.content = Write, req.FilePath, req.Content
	return op.executeLocalFile(ctx)
}

func (op localOperation) executeLocalFile(ctx context.Context) (string, *diff.Lines, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
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

func resolveRuntime(ctx context.Context, access runtimeAccess, name, raw string) (localOperation, error) {
	op := localOperation{name: name, runtime: access.runtime, operations: access.operations}
	files := access.operations.Files
	if name == Execute && access.operations.Shell == nil && access.operations.Commands == nil {
		return op, errors.New("所选工作区不支持命令执行")
	}
	if name != Execute && files == nil {
		return op, errors.New("所选工作区不支持文件操作")
	}
	var args localArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return op, err
	}
	identity := access.target.Identity()
	if session, ok := access.runtime.(project.ComputerSession); ok {
		identity += "/" + session.ComputerSessionID()
	}
	if name == Execute {
		if strings.TrimSpace(args.Command) == "" || len(args.Command) > maxLocalWrite || strings.ContainsRune(args.Command, 0) {
			return op, errors.New("需要有效的命令，且不能超过 16 KiB")
		}
		op.target, op.content, op.fileVersion = access.target.Directory, args.Command, identity
		return op, nil
	}
	if strings.TrimSpace(args.Path) == "" || strings.ContainsRune(args.Path, 0) {
		return op, errors.New("请指定路径")
	}
	var err error
	op.target, op.fileVersion, err = files.Resolve(ctx, args.Path, name == Write)
	if err != nil {
		return op, err
	}
	if name == Write {
		if args.Content == nil || len(*args.Content) > maxLocalWrite {
			return op, errors.New("待写文本必须明确提供，且不能超过 16 KiB")
		}
		op.content, op.exists = *args.Content, op.fileVersion != "missing"
	}
	root, _, err := files.Resolve(ctx, access.target.Directory, false)
	if err != nil {
		return op, err
	}
	target := op.target
	if access.runtime.Environment().OS == "windows" {
		root, target = strings.ToLower(root), strings.ToLower(target)
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if target == root || strings.HasPrefix(target, prefix) {
		op.root = root
		relative := ""
		if target != root {
			relative = strings.TrimPrefix(target, prefix)
		}
		for part := range strings.SplitSeq(relative, "/") {
			part = strings.ToLower(part)
			if strings.HasPrefix(part, ".") || strings.Contains(part, "secret") || strings.Contains(part, "credential") ||
				strings.Contains(part, "password") || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") {
				op.sensitive = true
			}
		}
	}
	op.fileVersion += "\x00" + identity
	return op, nil
}

func (op localOperation) executeRuntime(ctx context.Context) (string, *diff.Lines, error) {
	files := op.operations.Files
	target, version, err := files.Resolve(ctx, op.target, op.name == Write)
	approved, _, _ := strings.Cut(op.fileVersion, "\x00")
	if err != nil {
		return "", nil, err
	}
	if target != op.target || version != approved {
		return "", nil, errors.New("操作目标已变化，请重新请求批准")
	}
	ctx = project.WithWriteVersion(ctx, target, version)
	return executeFiles(ctx, files, op)
}

func executeFiles(ctx context.Context, backend project.FileSystem, op localOperation) (string, *diff.Lines, error) {
	target := op.target
	request := &filesystem.ReadRequest{FilePath: target}
	switch op.name {
	case Read:
		file, err := backend.Read(ctx, request)
		if err != nil {
			return "", nil, err
		}
		return file.Content, nil, nil
	case List:
		items, err := backend.LsInfo(ctx, &filesystem.LsInfoRequest{Path: target})
		if err != nil {
			return "", nil, err
		}
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.Path)
		}
		data, err := json.Marshal(names)
		return string(data), nil, err
	case Write:
		if local, ok := backend.(localFilesystem); ok {
			return local.write(ctx, &filesystem.WriteRequest{FilePath: target, Content: op.content})
		}
		before, readable := "", !op.exists
		if op.exists {
			if file, err := backend.Read(ctx, request); err == nil {
				before, readable = file.Content, true
			}
		}
		if err := backend.Write(ctx, &filesystem.WriteRequest{FilePath: target, Content: op.content}); err != nil {
			return "", nil, err
		}
		var changes *diff.Lines
		if readable {
			changes = diff.Compare(before, op.content)
		}
		return "已写入 " + target, changes, nil
	default:
		return "", nil, errors.New("不支持的文件操作")
	}
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
