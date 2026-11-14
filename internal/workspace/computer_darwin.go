//go:build darwin

package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ebitengine/purego"
	"golang.org/x/image/draw"
)

type quartzPoint struct{ X, Y float64 }
type quartzSize struct{ Width, Height float64 }
type quartzRect struct {
	Origin quartzPoint
	Size   quartzSize
}

type macComputer struct {
	canCapture, canPost func() bool
	mainDisplay         func() uint32
	displayBounds       func(uint32) quartzRect
	windowList          func(uint32, uint32) uintptr
	mouseEvent          func(uintptr, uint32, quartzPoint, uint32) uintptr
	keyEvent            func(uintptr, uint16, bool) uintptr
	setUnicode          func(uintptr, uintptr, *uint16)
	setFlags            func(uintptr, uint64)
	post                func(uint32, uintptr)
	release             func(uintptr)
	arrayCount          func(uintptr) int
	arrayValue          func(uintptr, int) uintptr
	dictionaryValue     func(uintptr, uintptr) uintptr
	makeString          func(uintptr, string, uint32) uintptr
	stringValue         func(uintptr, *byte, int, uint32) bool
	numberValue         func(uintptr, int32, *int64) bool
	mu                  sync.Mutex
}

var macDesktop = sync.OnceValue(loadMacComputer)

func NewLocalComputer() ComputerDriver {
	if driver := macDesktop(); driver != nil {
		return driver
	}
	return nil
}

func loadMacComputer() *macComputer {
	driver := &macComputer{}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		_ = purego.Dlclose(cg)
		return nil
	}
	for _, group := range []struct {
		library uintptr
		symbols map[string]any
	}{
		{cg, map[string]any{
			"CGPreflightScreenCaptureAccess": &driver.canCapture, "CGPreflightPostEventAccess": &driver.canPost,
			"CGMainDisplayID": &driver.mainDisplay, "CGDisplayBounds": &driver.displayBounds,
			"CGWindowListCopyWindowInfo": &driver.windowList, "CGEventCreateMouseEvent": &driver.mouseEvent,
			"CGEventCreateKeyboardEvent": &driver.keyEvent, "CGEventKeyboardSetUnicodeString": &driver.setUnicode,
			"CGEventSetFlags": &driver.setFlags, "CGEventPost": &driver.post,
		}},
		{cf, map[string]any{
			"CFRelease": &driver.release, "CFArrayGetCount": &driver.arrayCount,
			"CFArrayGetValueAtIndex": &driver.arrayValue, "CFDictionaryGetValue": &driver.dictionaryValue,
			"CFStringCreateWithCString": &driver.makeString, "CFStringGetCString": &driver.stringValue,
			"CFNumberGetValue": &driver.numberValue,
		}},
	} {
		for name, function := range group.symbols {
			symbol, err := purego.Dlsym(group.library, name)
			if err != nil {
				_ = purego.Dlclose(cf)
				_ = purego.Dlclose(cg)
				return nil
			}
			purego.RegisterFunc(function, symbol)
		}
	}
	return driver
}

func (d *macComputer) GetCapabilities(ctx context.Context) (ComputerCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return ComputerCapabilities{}, err
	}
	capture, input := d.canCapture(), d.canPost()
	return ComputerCapabilities{Screenshot: capture, ListWindows: capture, Click: input, TypeText: input, PressKey: input}, nil
}

func (d *macComputer) Screenshot(ctx context.Context) (Image, error) {
	if !d.canCapture() {
		return Image{}, errors.New("macOS Screen Recording permission is required")
	}
	dir, err := os.MkdirTemp("", "microlight-desktop-")
	if err != nil {
		return Image{}, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "screen.png")
	bounds := d.displayBounds(d.mainDisplay())
	width, height := int(bounds.Size.Width), int(bounds.Size.Height)
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 40_000_000 {
		return Image{}, errors.New("unsupported main display dimensions")
	}
	if err := exec.CommandContext(ctx, "/usr/sbin/screencapture", "-x", "-D", "1", "-t", "png", file).Run(); err != nil {
		return Image{}, fmt.Errorf("capture main display: %w", err)
	}
	f, err := os.Open(file)
	if err != nil {
		return Image{}, err
	}
	defer f.Close()
	config, err := png.DecodeConfig(io.LimitReader(f, 64<<20))
	if err != nil {
		return Image{}, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 80_000_000 {
		return Image{}, errors.New("unsupported captured display dimensions")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Image{}, err
	}
	pixels, err := png.Decode(io.LimitReader(f, 64<<20))
	if err != nil {
		return Image{}, err
	}
	if d.displayBounds(d.mainDisplay()) != bounds {
		return Image{}, errors.New("main display changed during capture")
	}
	// Match the image to Quartz coordinates on Retina displays.
	if pixels.Bounds().Dx() != width || pixels.Bounds().Dy() != height {
		logical := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.ApproxBiLinear.Scale(logical, logical.Bounds(), pixels, pixels.Bounds(), draw.Src, nil)
		pixels = logical
	}
	var result bytes.Buffer
	if err := png.Encode(&result, pixels); err != nil {
		return Image{}, err
	}
	return Image{MIMEType: "image/png", Data: result.Bytes(), X: int(bounds.Origin.X), Y: int(bounds.Origin.Y)}, ctx.Err()
}

func (d *macComputer) ListWindows(ctx context.Context) ([]WindowInfo, error) {
	if !d.canCapture() {
		return nil, errors.New("macOS Screen Recording permission is required")
	}
	const utf8Encoding = 0x08000100
	windowID := d.makeString(0, "kCGWindowNumber", utf8Encoding)
	title := d.makeString(0, "kCGWindowName", utf8Encoding)
	owner := d.makeString(0, "kCGWindowOwnerName", utf8Encoding)
	for _, key := range []uintptr{windowID, title, owner} {
		if key != 0 {
			defer d.release(key)
		}
	}
	if windowID == 0 || title == 0 || owner == 0 {
		return nil, errors.New("could not allocate window attributes")
	}
	list := d.windowList(1|16, 0) // On-screen windows, excluding desktop elements.
	if list == 0 {
		return nil, errors.New("macOS window list unavailable")
	}
	defer d.release(list)
	var windows []WindowInfo
	for i, count := 0, d.arrayCount(list); i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dict := d.arrayValue(list, i)
		var id int64
		if value := d.dictionaryValue(dict, windowID); value == 0 || !d.numberValue(value, 4, &id) {
			continue
		}
		read := func(key uintptr) string {
			value := d.dictionaryValue(dict, key)
			var text [4096]byte
			if value != 0 && d.stringValue(value, &text[0], len(text), utf8Encoding) {
				result, _, _ := strings.Cut(string(text[:]), "\x00")
				return result
			}
			return ""
		}
		name := read(title)
		if name == "" {
			name = read(owner)
		}
		if name != "" {
			windows = append(windows, WindowInfo{ID: strconv.FormatInt(id, 10), Title: name})
		}
	}
	return windows, nil
}

func (d *macComputer) postPair(ctx context.Context, down, up uintptr) error {
	for _, event := range []uintptr{down, up} {
		if event != 0 {
			defer d.release(event)
		}
	}
	if down == 0 || up == 0 {
		return errors.New("could not create macOS input events")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d.post(0, down)
	d.post(0, up)
	return nil
}

func (d *macComputer) Click(ctx context.Context, x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.canPost() {
		return errors.New("macOS Accessibility permission is required")
	}
	bounds := d.displayBounds(d.mainDisplay())
	point := quartzPoint{X: float64(x), Y: float64(y)}
	if point.X < bounds.Origin.X || point.Y < bounds.Origin.Y ||
		point.X >= bounds.Origin.X+bounds.Size.Width || point.Y >= bounds.Origin.Y+bounds.Size.Height {
		return errors.New("point is outside the main display")
	}
	return d.postPair(ctx, d.mouseEvent(0, 1, point, 0), d.mouseEvent(0, 2, point, 0))
}

func (d *macComputer) TypeText(ctx context.Context, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.canPost() {
		return errors.New("macOS Accessibility permission is required")
	}
	if len(text) == 0 || len(text) > 16<<10 || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return errors.New("invalid text input")
	}
	for _, char := range text {
		units := utf16.Encode([]rune{char})
		down, up := d.keyEvent(0, 0, true), d.keyEvent(0, 0, false)
		if down != 0 {
			d.setUnicode(down, uintptr(len(units)), &units[0])
		}
		if err := d.postPair(ctx, down, up); err != nil {
			return err
		}
	}
	return nil
}

var macKeys = map[string]uint16{
	"A": 0, "S": 1, "D": 2, "F": 3, "H": 4, "G": 5, "Z": 6, "X": 7, "C": 8, "V": 9,
	"B": 11, "Q": 12, "W": 13, "E": 14, "R": 15, "Y": 16, "T": 17, "1": 18, "2": 19, "3": 20,
	"4": 21, "6": 22, "5": 23, "9": 25, "7": 26, "8": 28, "0": 29, "O": 31, "U": 32, "I": 34,
	"P": 35, "ENTER": 36, "L": 37, "J": 38, "K": 40, "N": 45, "M": 46, "TAB": 48, "SPACE": 49,
	"BACKSPACE": 51, "ESC": 53, "F5": 96, "F6": 97, "F7": 98, "F3": 99, "F8": 100, "F9": 101,
	"F11": 103, "F10": 109, "F12": 111, "HOME": 115, "PAGEUP": 116, "DELETE": 117, "F4": 118,
	"END": 119, "F2": 120, "PAGEDOWN": 121, "F1": 122, "LEFT": 123, "RIGHT": 124, "DOWN": 125, "UP": 126,
}

func (d *macComputer) PressKey(ctx context.Context, shortcut string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.canPost() {
		return errors.New("macOS Accessibility permission is required")
	}
	parts := strings.Split(strings.ToUpper(shortcut), "+")
	var flags uint64
	for _, raw := range parts[:len(parts)-1] {
		var flag uint64
		switch strings.TrimSpace(raw) {
		case "SHIFT":
			flag = 1 << 17
		case "CTRL":
			flag = 1 << 18
		case "ALT", "OPTION":
			flag = 1 << 19
		case "CMD", "COMMAND", "WIN":
			flag = 1 << 20
		default:
			return fmt.Errorf("unsupported shortcut modifier %q", raw)
		}
		if flags&flag != 0 {
			return errors.New("duplicate shortcut modifier")
		}
		flags |= flag
	}
	name := strings.TrimSpace(parts[len(parts)-1])
	key, ok := macKeys[name]
	if !ok {
		return fmt.Errorf("unsupported desktop key %q", name)
	}
	down, up := d.keyEvent(0, key, true), d.keyEvent(0, key, false)
	for _, event := range []uintptr{down, up} {
		if event != 0 {
			d.setFlags(event, flags)
		}
	}
	return d.postPair(ctx, down, up)
}
