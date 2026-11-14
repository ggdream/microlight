//go:build windows

package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	desktopUser32    = windows.NewLazySystemDLL("user32.dll")
	desktopGDI32     = windows.NewLazySystemDLL("gdi32.dll")
	openInputDesktop = desktopUser32.NewProc("OpenInputDesktop")
	closeDesktop     = desktopUser32.NewProc("CloseDesktop")
	threadDesktop    = desktopUser32.NewProc("GetThreadDesktop")
	userObjectInfo   = desktopUser32.NewProc("GetUserObjectInformationW")
	threadDPI        = desktopUser32.NewProc("SetThreadDpiAwarenessContext")
	systemMetrics    = desktopUser32.NewProc("GetSystemMetrics")
	desktopDC        = desktopUser32.NewProc("GetDC")
	releaseDC        = desktopUser32.NewProc("ReleaseDC")
	compatibleDC     = desktopGDI32.NewProc("CreateCompatibleDC")
	createDIB        = desktopGDI32.NewProc("CreateDIBSection")
	selectObject     = desktopGDI32.NewProc("SelectObject")
	deleteObject     = desktopGDI32.NewProc("DeleteObject")
	deleteDC         = desktopGDI32.NewProc("DeleteDC")
	bitBlt           = desktopGDI32.NewProc("BitBlt")
	flushGDI         = desktopGDI32.NewProc("GdiFlush")
	enumWindows      = desktopUser32.NewProc("EnumWindows")
	isWindowVisible  = desktopUser32.NewProc("IsWindowVisible")
	windowText       = desktopUser32.NewProc("GetWindowTextW")
	sendInput        = desktopUser32.NewProc("SendInput")
)

type windowsComputer struct{}

func NewLocalComputer() ComputerDriver { return windowsComputer{} }

func desktopName(handle uintptr) string {
	var name [256]uint16
	const objectName = 2
	ok, _, _ := userObjectInfo.Call(handle, objectName, uintptr(unsafe.Pointer(&name[0])), unsafe.Sizeof(name), 0)
	if ok == 0 {
		return ""
	}
	return windows.UTF16ToString(name[:])
}

func onInputDesktop(ctx context.Context, action func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const readObjects = 1
	input, _, err := openInputDesktop.Call(0, 0, readObjects)
	if input == 0 {
		return fmt.Errorf("open input desktop: %w", err)
	}
	defer closeDesktop.Call(input)
	thread, _, _ := threadDesktop.Call(uintptr(windows.GetCurrentThreadId()))
	if name := desktopName(input); name == "" || name != desktopName(thread) {
		return errors.New("the interactive desktop is unavailable")
	}
	// DPI awareness is thread-scoped, shared across capture and input.
	const perMonitorV2 = ^uintptr(3)
	previous, _, err := threadDPI.Call(perMonitorV2)
	if previous == 0 {
		return fmt.Errorf("set desktop DPI awareness: %w", err)
	}
	defer threadDPI.Call(previous)
	return action()
}

func (windowsComputer) GetCapabilities(ctx context.Context) (ComputerCapabilities, error) {
	err := onInputDesktop(ctx, func() error { return nil })
	if err != nil {
		return ComputerCapabilities{}, ctx.Err()
	}
	return ComputerCapabilities{Screenshot: true, ListWindows: true, Click: true, TypeText: true, PressKey: true}, nil
}

func virtualScreen() image.Rectangle {
	metric := func(index uintptr) int {
		value, _, _ := systemMetrics.Call(index)
		return int(int32(value))
	}
	x, y, width, height := metric(76), metric(77), metric(78), metric(79)
	return image.Rect(x, y, x+width, y+height)
}

type bitmapHeader struct {
	size                             uint32
	width, height                    int32
	planes, bits                     uint16
	compression, imageSize           uint32
	xPixelsPerMeter, yPixelsPerMeter int32
	used, important                  uint32
}

func (windowsComputer) Screenshot(ctx context.Context) (result Image, err error) {
	var pixels *image.RGBA
	err = onInputDesktop(ctx, func() error {
		rect := virtualScreen()
		w, h := rect.Dx(), rect.Dy()
		if w <= 0 || h <= 0 || int64(w)*int64(h) > 40_000_000 {
			return errors.New("unsupported virtual desktop dimensions")
		}
		screen, _, err := desktopDC.Call(0)
		if screen == 0 {
			return fmt.Errorf("open screen DC: %w", err)
		}
		defer releaseDC.Call(0, screen)
		buffer, _, err := compatibleDC.Call(screen)
		if buffer == 0 {
			return fmt.Errorf("create capture DC: %w", err)
		}
		defer deleteDC.Call(buffer)
		header := bitmapHeader{size: uint32(unsafe.Sizeof(bitmapHeader{})), width: int32(w), height: -int32(h), planes: 1, bits: 32}
		var memory unsafe.Pointer
		bitmap, _, err := createDIB.Call(screen, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&memory)), 0, 0)
		if bitmap == 0 || memory == nil {
			return fmt.Errorf("create capture bitmap: %w", err)
		}
		defer deleteObject.Call(bitmap)
		previous, _, err := selectObject.Call(buffer, bitmap)
		if previous == 0 || previous == ^uintptr(0) {
			return fmt.Errorf("select capture bitmap: %w", err)
		}
		defer selectObject.Call(buffer, previous)
		const copyLayeredWindows = 0x40CC0020
		ok, _, err := bitBlt.Call(buffer, 0, 0, uintptr(w), uintptr(h), screen, uintptr(rect.Min.X), uintptr(rect.Min.Y), copyLayeredWindows)
		if ok == 0 {
			return fmt.Errorf("capture desktop: %w", err)
		}
		if ok, _, err := flushGDI.Call(); ok == 0 {
			return fmt.Errorf("flush desktop capture: %w", err)
		}
		source := unsafe.Slice((*byte)(memory), w*h*4)
		pixels = image.NewRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(source); i += 4 {
			pixels.Pix[i], pixels.Pix[i+1], pixels.Pix[i+2], pixels.Pix[i+3] = source[i+2], source[i+1], source[i], 255
		}
		result.X, result.Y = rect.Min.X, rect.Min.Y
		return nil
	})
	if err != nil {
		return Image{}, err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		return Image{}, err
	}
	result.MIMEType, result.Data = "image/png", encoded.Bytes()
	return result, ctx.Err()
}

var windowEnumerations sync.Map
var nextWindowEnumeration atomic.Uintptr

var collectWindow = windows.NewCallback(func(hwnd uintptr, state uintptr) uintptr {
	value, ok := windowEnumerations.Load(state)
	if !ok {
		return 0
	}
	list := value.(*[]WindowInfo)
	visible, _, _ := isWindowVisible.Call(hwnd)
	if visible != 0 {
		var title [1024]uint16
		n, _, _ := windowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if n > 0 {
			*list = append(*list, WindowInfo{ID: strconv.FormatUint(uint64(hwnd), 16), Title: windows.UTF16ToString(title[:n])})
		}
	}
	return 1
})

func (windowsComputer) ListWindows(ctx context.Context) (list []WindowInfo, err error) {
	err = onInputDesktop(ctx, func() error {
		token := nextWindowEnumeration.Add(1)
		windowEnumerations.Store(token, &list)
		defer windowEnumerations.Delete(token)
		ok, _, err := enumWindows.Call(collectWindow, token)
		if ok == 0 {
			return fmt.Errorf("enumerate desktop windows: %w", err)
		}
		return nil
	})
	return
}

type mouseEvent struct {
	x, y              int32
	data, flags, time uint32
	extra             uintptr
}

type keyEvent struct {
	key, scan   uint16
	flags, time uint32
	extra       uintptr
}

// INPUT holds a union whose size and alignment match MOUSEINPUT.
type desktopInput struct {
	kind  uint32
	value mouseEvent
}

func keyboardInput(event keyEvent) desktopInput {
	input := desktopInput{kind: 1}
	*(*keyEvent)(unsafe.Pointer(&input.value)) = event
	return input
}

func sendDesktopInput(input []desktopInput) error {
	count, _, err := sendInput.Call(uintptr(len(input)), uintptr(unsafe.Pointer(&input[0])), unsafe.Sizeof(input[0]))
	if count != uintptr(len(input)) {
		return fmt.Errorf("Windows accepted %d of %d input events: %w", count, len(input), err)
	}
	return nil
}

func (windowsComputer) Click(ctx context.Context, x, y int) error {
	return onInputDesktop(ctx, func() error {
		rect := virtualScreen()
		if !image.Pt(x, y).In(rect) {
			return errors.New("point is outside the virtual desktop")
		}
		normalized := func(offset, size int) int32 { return int32((int64(offset)*65536 + int64(size)/2) / int64(size)) }
		const absoluteVirtualMove = 0xC001
		const leftDown, leftUp = 2, 4
		return sendDesktopInput([]desktopInput{
			{value: mouseEvent{x: normalized(x-rect.Min.X, rect.Dx()), y: normalized(y-rect.Min.Y, rect.Dy()), flags: absoluteVirtualMove}},
			{value: mouseEvent{flags: leftDown}}, {value: mouseEvent{flags: leftUp}},
		})
	})
}

func (windowsComputer) TypeText(ctx context.Context, text string) error {
	if len(text) == 0 || len(text) > 16<<10 || strings.ContainsRune(text, 0) {
		return errors.New("invalid text input")
	}
	units := utf16.Encode([]rune(text))
	input := make([]desktopInput, 0, len(units)*2)
	for _, unit := range units {
		input = append(input, keyboardInput(keyEvent{scan: unit, flags: 4}), keyboardInput(keyEvent{scan: unit, flags: 6}))
	}
	return onInputDesktop(ctx, func() error { return sendDesktopInput(input) })
}

var namedDesktopKeys = map[string]uint16{
	"CTRL": 0x11, "ALT": 0x12, "SHIFT": 0x10, "WIN": 0x5B,
	"BACKSPACE": 8, "TAB": 9, "ENTER": 13, "ESC": 27, "SPACE": 32,
	"PAGEUP": 33, "PAGEDOWN": 34, "END": 35, "HOME": 36,
	"LEFT": 37, "UP": 38, "RIGHT": 39, "DOWN": 40, "INSERT": 45, "DELETE": 46,
}

func desktopKey(name string) (uint16, error) {
	if code := namedDesktopKeys[name]; code != 0 {
		return code, nil
	}
	if len(name) == 1 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= '0' && name[0] <= '9')) {
		return uint16(name[0]), nil
	}
	if suffix, ok := strings.CutPrefix(name, "F"); ok {
		if n, err := strconv.Atoi(suffix); err == nil && n >= 1 && n <= 24 {
			return uint16(0x6F + n), nil
		}
	}
	return 0, fmt.Errorf("unsupported desktop key %q", name)
}

func (windowsComputer) PressKey(ctx context.Context, keys string) error {
	names := strings.Split(strings.ToUpper(keys), "+")
	if len(names) > 5 {
		return errors.New("too many keys in shortcut")
	}
	input := make([]desktopInput, len(names)*2)
	seen := make(map[uint16]bool)
	for i, name := range names {
		key, err := desktopKey(strings.TrimSpace(name))
		if err != nil {
			return err
		}
		if seen[key] {
			return errors.New("duplicate shortcut key")
		}
		seen[key] = true
		var flags uint32
		if (key >= 33 && key <= 40) || key == 45 || key == 46 || key == 0x5B {
			flags = 1
		}
		input[i] = keyboardInput(keyEvent{key: key, flags: flags})
		input[len(input)-1-i] = keyboardInput(keyEvent{key: key, flags: flags | 2})
	}
	return onInputDesktop(ctx, func() error { return sendDesktopInput(input) })
}
