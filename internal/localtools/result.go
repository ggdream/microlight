package localtools

import (
	"context"

	"github.com/ggdream/microlight/pkg/diff"
)

type Result struct {
	CallID string
	Target string
	Text   string
	Diff   *diff.Lines
}

type reportToolResultKey struct{}

type MCPTarget struct {
	Label, Version string
}

func WithResultReporter(ctx context.Context, report func(Result)) context.Context {
	return context.WithValue(ctx, reportToolResultKey{}, report)
}
