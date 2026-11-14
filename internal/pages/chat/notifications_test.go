package chat

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplyNotificationShowsContentAndQuestion(t *testing.T) {
	reply := &turn{State: "done", message: message{Content: "Hello\n\nworld"}}
	if _, body := replyNotification(reply); body != "Hello world" {
		t.Fatalf("reply preview = %q", body)
	}
	reply.State = "waiting"
	reply.Clarifications = []clarification{{Question: "Which language?", Options: []string{"Go", "Rust"}}}
	if _, body := replyNotification(reply); body != "Which language?" {
		t.Fatalf("question preview = %q", body)
	}
	if got := notificationText(strings.Repeat("🌕", 130)); got != strings.Repeat("🌕", 119)+"…" {
		t.Fatalf("truncated preview = %q", got)
	}
}

func TestAppLogsCopyErrorsToBothFiles(t *testing.T) {
	dir := t.TempDir()
	closeLogs, err := openAppLogs(dir)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("working normally")
	slog.Error("operation failed")
	closeLogs()
	for _, name := range []string{"app.log", "error.log"} {
		data, err := os.ReadFile(filepath.Join(dir, "logs", name))
		if err != nil {
			t.Fatal(err)
		}
		if log := string(data); !strings.Contains(log, "operation failed") || strings.Contains(log, "working normally") != (name == "app.log") {
			t.Fatalf("unexpected %s content: %s", name, log)
		}
	}
}
