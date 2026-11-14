package workspace

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testDesktop struct {
	ComputerDriver
	clicks        int
	text          string
	key           string
	clickDisabled atomic.Bool
}

func (d *testDesktop) GetCapabilities(context.Context) (ComputerCapabilities, error) {
	return ComputerCapabilities{true, true, !d.clickDisabled.Load(), true, true}, nil
}
func (*testDesktop) Screenshot(context.Context) (Image, error) {
	return Image{MIMEType: "image/png", Data: []byte("pixels"), X: -10}, nil
}
func (*testDesktop) ListWindows(context.Context) ([]WindowInfo, error) {
	return []WindowInfo{{ID: "1", Title: "test window"}}, nil
}
func (d *testDesktop) Click(_ context.Context, x, y int) error       { d.clicks += x + y; return nil }
func (d *testDesktop) TypeText(_ context.Context, text string) error { d.text = text; return nil }
func (d *testDesktop) PressKey(_ context.Context, key string) error  { d.key = key; return nil }

func TestRemoteDesktopSessionAndOperations(t *testing.T) {
	const token = "desktop-test-token-00000000000000"
	driver := &testDesktop{}
	server := httptest.NewServer(DesktopHandler(driver, "desktop-session", token))
	defer server.Close()
	target := Target{Kind: "desktop", Resource: server.URL}
	ctx := t.Context()
	if _, err := Open(ctx, target, strings.Repeat("x", 32)); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("incorrect token: %v", err)
	}
	runtime, err := Open(ctx, target, token)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ops, err := DiscoverOperations(ctx, runtime)
	if err != nil || ops.Computer == nil || ops.Files != nil || ops.Shell != nil || ops.Browser != nil {
		t.Fatalf("desktop operations: %+v, %v", ops, err)
	}
	d := ops.Computer.(*remoteDesktop)
	if d.ComputerSessionID() != "desktop-session" {
		t.Fatal(d.ComputerSessionID())
	}
	image, err := d.Screenshot(ctx)
	if err != nil || image.X != -10 || image.MIMEType != "image/png" || string(image.Data) != "pixels" {
		t.Fatalf("screenshot: %+v, %v", image, err)
	}
	windows, err := d.ListWindows(ctx)
	if err != nil || len(windows) != 1 || windows[0].Title != "test window" {
		t.Fatalf("windows: %+v, %v", windows, err)
	}
	if err := d.Click(ctx, -4, 8); err != nil {
		t.Fatal(err)
	}
	if err := d.TypeText(ctx, "远程桌面"); err != nil {
		t.Fatal(err)
	}
	if err := d.PressKey(ctx, "Enter"); err != nil {
		t.Fatal(err)
	}
	driver.clickDisabled.Store(true)
	if err := d.Click(ctx, 40, 80); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("disabled click: %v", err)
	}
	d.info.SessionID = "previous-desktop-session"
	if err := d.Click(ctx, 40, 80); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("stale desktop session: %v", err)
	}
	server.Close()
	if driver.clicks != 4 || driver.text != "远程桌面" || driver.key != "Enter" {
		t.Fatalf("desktop calls: %+v", driver)
	}
}

func TestRemoteDesktopRejectsIncompleteClick(t *testing.T) {
	const token = "desktop-test-token-00000000000000"
	driver := &testDesktop{}
	handler := DesktopHandler(driver, "desktop-session", token)
	for _, body := range []string{`{}`, `{"X":0}`, `{"X":0,"Y":0}{}`, `{"X":0,"Y":0,"typo":true}`} {
		req := httptest.NewRequest(http.MethodPost, desktopPath+"click", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(desktopSessionHeader, "desktop-session")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("body %q returned %d", body, res.Code)
		}
	}
}

func TestRemoteDesktopDoesNotReplayInputAfterLostResponse(t *testing.T) {
	const token = "desktop-test-token-00000000000000"
	var clicks atomic.Int32
	handler := DesktopHandler(&testDesktop{}, "desktop-session", token)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != desktopPath+"click" {
			handler.ServeHTTP(w, r)
			return
		}
		clicks.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	runtime, err := Open(t.Context(), Target{Kind: "desktop", Resource: server.URL}, token)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	err = runtime.(ComputerProvider).Computer().Click(t.Context(), 2, 3)
	if err == nil || !strings.Contains(err.Error(), "outcome unknown") || clicks.Load() != 1 {
		t.Fatalf("lost acknowledgement: %v, clicks %d", err, clicks.Load())
	}
}

func TestRemoteDesktopNativeScreenshot(t *testing.T) {
	driver := NewLocalComputer()
	if driver == nil {
		t.Skip("native desktop driver unavailable")
	}
	caps, err := driver.GetCapabilities(t.Context())
	if err != nil || !caps.Screenshot {
		t.Skipf("interactive desktop unavailable: %v", err)
	}
	const token = "desktop-test-token-00000000000000"
	server := httptest.NewServer(DesktopHandler(driver, "native-desktop", token))
	defer server.Close()
	runtime, err := Open(t.Context(), Target{Kind: "desktop", Resource: server.URL}, token)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	d := runtime.(ComputerProvider).Computer()
	image, err := d.Screenshot(t.Context())
	if err != nil || image.MIMEType != "image/png" || len(image.Data) == 0 {
		t.Fatalf("native screenshot: %v", err)
	}
	if _, err := d.ListWindows(t.Context()); err != nil {
		t.Fatal(err)
	}
}
