package activity

import (
	"bytes"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/diff"

	"github.com/cloudwego/eino/schema"
)

type Entry struct {
	Text           string      `json:",omitempty"`
	Tool           string      `json:",omitempty"`
	Target         string      `json:",omitempty"`
	Arguments      string      `json:",omitempty"`
	CallID         string      `json:",omitempty"`
	Diff           *diff.Lines `json:",omitempty"`
	ResolvedTarget string      `json:",omitempty"`
	Output         *string     `json:",omitempty"`
	Pending        bool        `json:",omitempty"`
}

const SkillTool = "skill"
const AskUserTool = "ask_user"

func DescribeToolCall(call schema.ToolCall) Entry {
	var args struct {
		Path        string
		URL         string
		Command     string
		Description string
		Skill       string
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	name, target := call.Function.Name, args.Command
	switch name {
	case localtools.Read, localtools.List, localtools.Write:
		target = args.Path
	case localtools.Fetch:
		target = args.URL
	case SkillTool:
		target = args.Skill
	default:
		if target == "" {
			target = args.Description
		}
	}
	return Entry{Tool: name, Target: strings.TrimSpace(target), Arguments: strings.TrimSpace(call.Function.Arguments), CallID: call.ID, Pending: call.ID != ""}
}

func (activity Entry) Label(target string) string {
	label := activity.Tool
	switch activity.Tool {
	case localtools.Execute:
		label = locales.RunCommand()
	case localtools.Read:
		label = locales.ReadFile()
	case localtools.List:
		label = locales.ListFolder()
	case localtools.Write:
		label = locales.WriteFile()
	case localtools.Fetch:
		label = locales.VisitURL()
	case AskUserTool:
		label = locales.AskUserTool()
	case SkillTool:
		label = locales.UseSkill()
	case "write_todos":
		label = locales.UpdateTasksTool()
	}
	if target != "" {
		return label + " " + target
	}
	return label
}

func (activity Entry) Icon() string {
	switch activity.Tool {
	case localtools.Read:
		return "book_open"
	case localtools.Write:
		return "edit"
	case localtools.List:
		return "folder"
	case localtools.Fetch:
		return "globe"
	case AskUserTool:
		return "question"
	case SkillTool:
		return "skill"
	case "write_todos":
		return "update_tasks"
	}
	name := strings.ToLower(activity.Tool)
	switch {
	case activity.RunsCommand():
		return "window_console"
	case strings.Contains(name, "search"):
		return "search"
	default:
		return "wrench"
	}
}

func (activity Entry) RunsCommand() bool {
	name := strings.ToLower(activity.Tool)
	switch name {
	case localtools.Read, localtools.List, localtools.Write, localtools.Fetch, AskUserTool, SkillTool, "write_todos":
		return false
	}
	if strings.Contains(name, "search") {
		return false
	}
	isMCP := strings.HasPrefix(name, "mcp_")
	if isMCP {
		name = name[:strings.LastIndex(name, "_")]
	}
	for _, commandTool := range []string{localtools.Execute, "bash", "shell", "terminal", "exec", "run_command", "exec_command", "execute_command", "shell_command", "run_shell", "run_shell_command"} {
		if name == commandTool || (isMCP && strings.HasSuffix(name, "_"+commandTool)) {
			return true
		}
	}
	return activity.CommandText() != ""
}

func (activity Entry) CommandText() string {
	var args struct {
		Command string
		Cmd     string
	}
	_ = json.Unmarshal([]byte(activity.Arguments), &args)
	if strings.TrimSpace(args.Command) != "" {
		return args.Command
	}
	return args.Cmd
}

func (activity Entry) ShortTarget() string {
	if activity.Tool == SkillTool {
		return activity.SkillName()
	}
	target := activity.Target
	if target == "" {
		return ""
	}
	switch activity.Tool {
	case localtools.Read, localtools.List, localtools.Write:
		target = filepath.Base(target)
	case localtools.Fetch:
		if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
			target = parsed.Host
		}
	}
	target = strings.Join(strings.Fields(target), " ")
	const maxRunes = 36
	if chars := []rune(target); len(chars) > maxRunes {
		return string(chars[:maxRunes-1]) + "…"
	}
	return target
}

func (activity Entry) SkillName() string {
	if activity.Target != "" {
		return activity.Target
	}
	// Older records kept the name only in the tool arguments.
	var args struct{ Skill string }
	_ = json.Unmarshal([]byte(activity.Arguments), &args)
	return strings.TrimSpace(args.Skill)
}

func (activity Entry) Details() string {
	if activity.Arguments == "" {
		return activity.Target
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, []byte(activity.Arguments), "", "  "); err == nil {
		return formatted.String()
	}
	return activity.Arguments
}

func (activity Entry) DisplayTarget() string {
	if activity.ResolvedTarget != "" {
		return activity.ResolvedTarget
	}
	return activity.Target
}
