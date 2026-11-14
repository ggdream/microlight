//go:build windows

package workspace

import (
	"bytes"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestLocalComputerReadOnly(t *testing.T) {
	ctx := t.Context()
	driver := NewLocalComputer()
	caps, err := driver.GetCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !caps.Screenshot || !caps.ListWindows {
		t.Skip("interactive desktop unavailable")
	}
	if _, err := driver.ListWindows(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := driver.Screenshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.Data))
	if err != nil || result.MIMEType != "image/png" || config.Width < 1 || config.Height < 1 {
		t.Fatalf("desktop screenshot = %+v, %v", config, err)
	}
	if err := driver.Click(ctx, 1<<30, 1<<30); err == nil {
		t.Fatal("accepted a click outside the virtual desktop")
	}
}

func TestLocalComputerInput(t *testing.T) {
	if os.Getenv("MICROLIGHT_TEST_DESKTOP_INPUT") != "1" {
		t.Skip("requires an interactive desktop and moves the pointer")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	createWindow := desktopUser32.NewProc("CreateWindowExW")
	destroyWindow := desktopUser32.NewProc("DestroyWindow")
	focus := desktopUser32.NewProc("SetFocus")
	setForeground := desktopUser32.NewProc("SetForegroundWindow")
	foreground := desktopUser32.NewProc("GetForegroundWindow")
	getFocus := desktopUser32.NewProc("GetFocus")
	peek := desktopUser32.NewProc("PeekMessageW")
	translate := desktopUser32.NewProc("TranslateMessage")
	dispatch := desktopUser32.NewProc("DispatchMessageW")
	previous, _, _ := foreground.Call()
	defer setForeground.Call(previous)
	class := windows.StringToUTF16Ptr("Edit")
	title := windows.StringToUTF16Ptr("Microlight Desktop Test")
	hwnd, _, err := createWindow.Call(8, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0x10CF0080, 140, 140, 480, 200, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	defer destroyWindow.Call(hwnd)
	setForeground.Call(hwnd)
	if current, _, _ := foreground.Call(); current != hwnd {
		thread, _, _ := desktopUser32.NewProc("GetWindowThreadProcessId").Call(current, 0)
		self := uintptr(windows.GetCurrentThreadId())
		attach := desktopUser32.NewProc("AttachThreadInput")
		if thread != 0 && thread != self {
			if ok, _, _ := attach.Call(self, thread, 1); ok != 0 {
				setForeground.Call(hwnd)
				attach.Call(self, thread, 0)
			}
		}
		if current, _, _ := foreground.Call(); current != hwnd {
			t.Skip("Windows did not grant focus to the test window")
		}
	}
	focus.Call(hwnd)
	pump := func() {
		var message [64]byte
		until := time.Now().Add(150 * time.Millisecond)
		for time.Now().Before(until) {
			pointer := uintptr(unsafe.Pointer(&message[0]))
			if ok, _, _ := peek.Call(pointer, 0, 0, 0, 1); ok != 0 {
				translate.Call(pointer)
				dispatch.Call(pointer)
			} else {
				time.Sleep(time.Millisecond)
			}
		}
	}
	pump()
	driver := NewLocalComputer()
	type point struct{ x, y int32 }
	var cursor point
	getCursor := desktopUser32.NewProc("GetCursorPos")
	setCursor := desktopUser32.NewProc("SetCursorPos")
	getCursor.Call(uintptr(unsafe.Pointer(&cursor)))
	defer setCursor.Call(uintptr(cursor.x), uintptr(cursor.y))
	var position point
	clientToScreen := desktopUser32.NewProc("ClientToScreen")
	position.x, position.y = 24, 24
	clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&position)))
	if err := driver.Click(t.Context(), int(position.x), int(position.y)); err != nil {
		t.Fatal(err)
	}
	pump()
	if focused, _, _ := getFocus.Call(); focused != hwnd {
		t.Fatal("test edit window lost focus")
	}
	// The classic Win32 Edit control does not implement Ctrl+A.
	if err := driver.PressKey(t.Context(), "Home"); err != nil {
		t.Fatal(err)
	}
	pump()
	if err := driver.PressKey(t.Context(), "Shift+End"); err != nil {
		t.Fatal(err)
	}
	pump()
	const text = "微光 🌕"
	if err := driver.TypeText(t.Context(), text); err != nil {
		t.Fatal(err)
	}
	pump()
	var result [128]uint16
	windowText.Call(hwnd, uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)))
	if got := windows.UTF16ToString(result[:]); got != text {
		t.Fatalf("typed text = %q, want %q", got, text)
	}
}

func TestWin32DesktopInputLayout(t *testing.T) {
	want := uintptr(28)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 40
	}
	if size := unsafe.Sizeof(desktopInput{}); size != want {
		t.Errorf("INPUT size = %d, want %d", size, want)
	}
}
