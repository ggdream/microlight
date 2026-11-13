package localtools

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

func TestLocalShellTimeoutStopsForegroundProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Bash")
	}
	dir := t.TempDir()
	const command = "sleep 20 & wait"
	ctx := context.WithValue(t.Context(), approvedOperationKey{}, localOperation{name: Execute, target: dir, content: command})
	timeout := 100 * time.Millisecond
	start := time.Now()
	result, err := (localCommandShell{}).Execute(ctx, &filesystem.ExecuteRequest{Command: command, Timeout: &timeout})
	if err != nil || result == nil || !result.TimedOut || time.Since(start) > 3*time.Second {
		t.Fatalf("expected a bounded foreground timeout: %+v, %v", result, err)
	}
}
