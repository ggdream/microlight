package localtools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/pkg/asr"
)

func TestASRIsOptInAndUsesAuthorizedAttachment(t *testing.T) {
	if tool, _, err := NewASRTool(ASRConfig{}, nil); err != nil || tool != nil {
		t.Fatal("ASR should be disabled by default")
	}
	config := ASRConfig{Enabled: true, Provider: asr.MiMo, Options: map[asr.ID]asr.Options{asr.MiMo: {APIKey: "secret"}}}
	calls := 0
	tool, target, err := NewASRTool(config, ocrTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		r.Body.Close()
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"queue_full","message":"please retry secret"}}`))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.InvokableRun(t.Context(), `{"path":"a.mp3"}`); err == nil || calls != 0 {
		t.Fatal("unapproved file sent")
	}
	middleware := Middleware{Mode: Ask, ASRTarget: target, EncodeApproval: func(question string, a Approval) ([]byte, error) { return json.Marshal(a) }}
	wrapped, err := middleware.WrapInvokableToolCall(t.Context(), tool.InvokableRun, &adk.ToolContext{Name: ASR, CallID: "asr-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithToolAttachments(t.Context(), []*attachment.File{{Name: "sample.mp3", Data: []byte("fixture")}})
	if _, err := wrapped(ctx, `{"attachment_index":1}`); err == nil || calls != 0 {
		t.Fatal("audio upload should pause for approval")
	}
	middleware.Mode = Full
	wrapped, _ = middleware.WrapInvokableToolCall(t.Context(), tool.InvokableRun, &adk.ToolContext{Name: ASR, CallID: "asr-2"})
	var report Result
	ctx = WithResultReporter(ctx, func(r Result) { report = r })
	text, err := wrapped(ctx, `{"attachment_index":1}`)
	if err != nil || calls != 1 || !strings.Contains(text, "queue_full") || strings.Contains(text, "retry secret") || report.Text != text {
		t.Fatalf("incorrect result: %s %v", text, err)
	}
}
func TestASRResolvesWorkspaceAndAttachments(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	if err := os.WriteFile(filepath.Join(dir, "sample.mp3"), []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	middleware := Middleware{Workspace: dir}
	op, err := middleware.resolveASR(t.Context(), `{"path":"sample.mp3"}`)
	if err != nil || string(op.audioData) != "audio" || op.root != dir {
		t.Fatal("workspace file resolution failed", err)
	}
	for _, raw := range []string{`{}`, `{"path":"sample.mp3","attachment_index":1}`, `{"attachment_index":0}`} {
		if _, err := middleware.resolveASR(context.Background(), raw); err == nil {
			t.Fatal("invalid arguments accepted", raw)
		}
	}
}
