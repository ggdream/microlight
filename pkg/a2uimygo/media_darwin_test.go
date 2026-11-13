package a2uimygo_test

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/ui"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uimygo"
)

//go:embed testdata/native-video.mp4
var nativeVideo []byte

func init() { runtime.LockOSThread() }

func TestMain(m *testing.M) {
	// AVFoundation loads assets through the main dispatch queue.
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		panic(err)
	}
	var runLoop func()
	purego.RegisterLibFunc(&runLoop, cf, "CFRunLoopRun")
	go func() { os.Exit(m.Run()) }()
	runLoop()
}

func TestNativeMP4Playback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(nativeVideo)
	}))
	defer server.Close()

	message, err := a2ui.Decode([]byte(fmt.Sprintf(
		`{"version":"1.0","createSurface":{"surfaceId":"test","components":[{"id":"root","component":"Video","url":%q}]}}`,
		server.URL+"/video.mp4",
	)))
	if err != nil {
		t.Fatal(err)
	}
	var processor a2ui.Processor
	if err := processor.Apply(message); err != nil {
		t.Fatal(err)
	}
	visible := true
	var mediaErr error
	tester := ui.NewTester(func(c *ui.Context) {
		if visible {
			a2uimygo.View(c, processor.Surface("test"), a2uimygo.Options{
				Media:   a2uimygo.MediaOptions{Client: server.Client(), RequireActivation: true},
				OnError: func(err error) { mediaErr = err },
			})
		}
	}, 320, 320)
	t.Cleanup(func() {
		visible = false
		tester.Frame()
	})
	if requests.Load() != 0 {
		t.Fatal("remote video was requested before activation")
	}
	if err := tester.Click("Load media"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		tester.Frame()
		if mediaErr != nil {
			t.Fatal(mediaErr)
		}
		if err := tester.Click("Play"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("MP4 did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tester.Frame()
		if _, playing := tester.Find("Pause"); !playing {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		frame := tester.Image()
		for y := 0; y < frame.Bounds().Dy(); y++ {
			pixel := frame.RGBAAt(100, y)
			if pixel.R > 200 && pixel.G < 40 && pixel.B < 40 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("MP4 did not render a red video frame")
}
