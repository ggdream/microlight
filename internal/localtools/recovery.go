package localtools

import (
	"context"
	"encoding/json"
	"strings"
)

type RecoveryCall struct {
	Tool, Arguments string
	Result          *string
}

type recoveryKey struct{}

func RecordsMutation(name string) bool {
	return name == Write || name == Execute || name == ExecuteBackground || name == BrowserUse || name == OCR || name == ParseDocument || strings.HasPrefix(name, "mcp_")
}

func WithRecovery(ctx context.Context, calls []RecoveryCall) context.Context {
	return context.WithValue(ctx, recoveryKey{}, calls)
}

func recoveryNeedsApproval(ctx context.Context, op localOperation) bool {
	_, recovering := ctx.Value(recoveryKey{}).([]RecoveryCall)
	return recovering && (op.mcp || RecordsMutation(op.name))
}

func recoveryResult(ctx context.Context, op localOperation, raw string) (string, bool) {
	if !recoveryNeedsApproval(ctx, op) {
		return "", false
	}
	calls := ctx.Value(recoveryKey{}).([]RecoveryCall)
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if call.Tool != op.name || comparableArguments(call.Arguments) != comparableArguments(raw) {
			continue
		}
		if call.Result != nil {
			return "Result recorded before the interruption; this action was not run again:\n" + *call.Result, true
		}
		// An unknown operation can have side effects. Let the normal approval flow
		// decide if it should run again, including in Full mode.
		return "", false
	}
	return "", false
}

func comparableArguments(raw string) string {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) == nil {
		if normalized, err := json.Marshal(value); err == nil {
			return string(normalized)
		}
	}
	return raw
}
