package localtools

import (
	"context"

	"github.com/ggdream/microlight/pkg/diff"
)

type Result struct {
	CallID  string
	Target  string
	Text    string
	Diff    *diff.Lines
	Command *CommandResult
}

type CommandResult struct {
	ExitCode  *int `json:",omitempty"`
	TimedOut  bool `json:",omitempty"`
	Truncated bool `json:",omitempty"`
	Failed    bool `json:",omitempty"`
}

type reportToolResultKey struct{}

type MCPTarget struct {
	Label, Version string
}

type DriverTarget struct {
	Label, Identity, Title string
}

func WithResultReporter(ctx context.Context, report func(Result)) context.Context {
	return context.WithValue(ctx, reportToolResultKey{}, report)
}
