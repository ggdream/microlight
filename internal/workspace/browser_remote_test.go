package workspace

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemoteBrowserRetainsTabOnDisconnect(t *testing.T) {
	local := NewLocalBrowser()
	if local == nil {
		t.Skip("Chromium executable unavailable")
	}
	profile := t.TempDir()
	ctx := t.Context()
	cmd := exec.CommandContext(ctx, local.executable, "--headless", "--no-first-run", "--disable-background-networking",
		"--remote-debugging-port=0", "--user-data-dir="+profile, "about:blank")
	hideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	var endpoint string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		fields := strings.Fields(string(data))
		if len(fields) == 2 {
			endpoint = "ws://127.0.0.1:" + fields[0] + fields[1]
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if endpoint == "" {
		t.Fatal("Chromium did not publish its debugging endpoint")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>Remote browser</title><button id="go" onclick="this.textContent='Clicked'">Ready</button>`))
	}))
	defer server.Close()
	connect := func() *remoteBrowserRuntime {
		t.Helper()
		runtime, err := Open(ctx, Target{Kind: "browser", Resource: endpoint}, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { runtime.Close() })
		return runtime.(*remoteBrowserRuntime)
	}
	first := connect()
	tabs, err := first.ListTabs(ctx)
	if err != nil || len(tabs) != 1 {
		t.Fatalf("tabs = %+v, %v", tabs, err)
	}
	id := tabs[0].ID
	if err := first.Navigate(ctx, id, server.URL); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second := connect()
	if first.BrowserSessionID() != second.BrowserSessionID() {
		t.Fatal("browser identity changed after reconnect")
	}
	if err := second.Click(ctx, id, "#go"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := second.Snapshot(ctx, id)
	if err != nil || snapshot.Title != "Remote browser" || !strings.Contains(snapshot.Text, "Clicked") {
		t.Fatalf("snapshot = %+v, %v", snapshot, err)
	}
}
