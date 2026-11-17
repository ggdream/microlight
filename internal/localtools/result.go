package localtools

import (
	"context"

	"github.com/ggdream/microlight/pkg/diff"
)

type Result struct {
	CallID   string
	Target   string
	Model    string
	Text     string
	Diff     *diff.Lines
	Command  *CommandResult
	Conflict bool
}

type CommandResult struct {
	ExitCode  *int `json:",omitempty"`
	TimedOut  bool `json:",omitempty"`
	Truncated bool `json:",omitempty"`
	Failed    bool `json:",omitempty"`
}

func (result CommandResult) Succeeded() bool {
	return !result.Failed && !result.TimedOut && result.ExitCode != nil && *result.ExitCode == 0
}

type reportToolResultKey struct{}

type MCPTarget struct {
	Label, Version string
}

func WithResultReporter(ctx context.Context, report func(Result)) context.Context {
	return context.WithValue(ctx, reportToolResultKey{}, report)
}

type toolStartReporterKey struct{}

func WithToolStartReporter(ctx context.Context, report func(string) error) context.Context {
	return context.WithValue(ctx, toolStartReporterKey{}, report)
}
