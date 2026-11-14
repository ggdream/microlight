package workspace

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalBrowserKeepsTabAcrossCalls(t *testing.T) {
	browser := NewLocalBrowser()
	if browser == nil {
		t.Skip("Chromium executable unavailable")
	}
	t.Cleanup(browser.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>Browser test</title><button id="action" onclick="document.querySelector('output').textContent='Clicked'">Run</button><output>Ready</output>`))
	}))
	t.Cleanup(server.Close)
	ctx := t.Context()
	tabs, err := browser.ListTabs(ctx)
	if err != nil || len(tabs) != 1 {
		t.Fatalf("initial tabs = %+v, %v", tabs, err)
	}
	id := tabs[0].ID
	if err := browser.Navigate(ctx, id, server.URL); err != nil {
		t.Fatal(err)
	}
	before, err := browser.Snapshot(ctx, id)
	if err != nil || before.Title != "Browser test" || !strings.Contains(before.Text, "Ready") {
		t.Fatalf("initial snapshot = %+v, %v", before, err)
	}
	if len(before.Controls) != 1 || before.Controls[0].Label != "Run" {
		t.Fatalf("visible controls = %+v", before.Controls)
	}
	if err := browser.Click(ctx, id, before.Controls[0].Selector); err != nil {
		t.Fatal(err)
	}
	after, err := browser.Snapshot(ctx, id)
	if err != nil || !strings.Contains(after.Text, "Clicked") {
		t.Fatalf("snapshot after click = %+v, %v", after, err)
	}
}
