package localtools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/adk"
)

func TestStaleWorkspaceWriteReportsConflictAndPreservesNewContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "shared.txt")
	if err := os.WriteFile(target, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	policy := Middleware{Workspace: dir, Mode: Full}
	var conflict Result
	access := NewAccess(dir, Full)
	access.TrackFiles(&FileVersions{})
	ctx := WithResultReporter(WithAccess(t.Context(), access), func(result Result) { conflict = result })
	read, err := policy.WrapInvokableToolCall(ctx, Tool(Read).InvokableRun, &adk.ToolContext{Name: Read, CallID: "read-before"})
	if err != nil {
		t.Fatal(err)
	}
	write, err := policy.WrapInvokableToolCall(ctx, Tool(Write).InvokableRun, &adk.ToolContext{Name: Write, CallID: "write-after"})
	if err != nil {
		t.Fatal(err)
	}
	if text, err := read(ctx, `{"path":"shared.txt"}`); err != nil || text != "first" {
		t.Fatalf("read: %q, %v", text, err)
	}
	if err := os.WriteFile(target, []byte("other conversation's update"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := write(ctx, `{"path":"shared.txt","content":"our update"}`); err != nil {
		t.Fatal(err)
	}
	if !conflict.Conflict || filepath.Base(conflict.Target) != "shared.txt" {
		t.Fatalf("missing file conflict: %+v", conflict)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "other conversation's update" {
		t.Fatalf("stale call overwrote newer content: %q, %v", got, err)
	}
	if _, err := read(ctx, `{"path":"shared.txt"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := write(ctx, `{"path":"shared.txt","content":"our update"}`); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "our update" {
		t.Fatalf("write after reread: %q, %v", got, err)
	}
}
