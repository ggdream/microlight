package proxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

type cfProxy struct {
	settings                  func() uintptr
	proxiesForURL             func(uintptr, uintptr) uintptr
	executeURL, executeScript func(uintptr, uintptr, uintptr, *cfPACContext) uintptr
	makeString                func(uintptr, string, uint32) uintptr
	makeURL                   func(uintptr, uintptr, uintptr) uintptr
	release                   func(uintptr)
	retain                    func(uintptr) uintptr
	dictGet                   func(uintptr, uintptr) uintptr
	arrayCount                func(uintptr) int
	arrayGet                  func(uintptr, int) uintptr
	equal                     func(uintptr, uintptr) bool
	stringLen                 func(uintptr) int
	stringGet                 func(uintptr, []byte, int, uint32) bool
	numberGet                 func(uintptr, int, *int32) bool
	runLoop                   func() uintptr
	addSource                 func(uintptr, uintptr, uintptr)
	invalidateSource          func(uintptr)
	runInMode                 func(uintptr, float64, bool) int32
	keys                      map[string]uintptr
	callback                  uintptr
}

var macProxy = sync.OnceValues(func() (*cfProxy, error) {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	nw, err := purego.Dlopen("/System/Library/Frameworks/CFNetwork.framework/CFNetwork", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	p := &cfProxy{keys: make(map[string]uintptr)}
	for name, fn := range map[string]any{
		"CFStringCreateWithCString": &p.makeString, "CFURLCreateWithString": &p.makeURL,
		"CFRelease": &p.release, "CFRetain": &p.retain, "CFEqual": &p.equal,
		"CFDictionaryGetValue": &p.dictGet, "CFArrayGetCount": &p.arrayCount, "CFArrayGetValueAtIndex": &p.arrayGet,
		"CFStringGetLength": &p.stringLen, "CFStringGetCString": &p.stringGet, "CFNumberGetValue": &p.numberGet,
		"CFRunLoopGetCurrent": &p.runLoop, "CFRunLoopAddSource": &p.addSource,
		"CFRunLoopSourceInvalidate": &p.invalidateSource, "CFRunLoopRunInMode": &p.runInMode,
	} {
		purego.RegisterLibFunc(fn, cf, name)
	}
	for name, fn := range map[string]any{
		"CFNetworkCopySystemProxySettings": &p.settings, "CFNetworkCopyProxiesForURL": &p.proxiesForURL,
		"CFNetworkExecuteProxyAutoConfigurationURL":    &p.executeURL,
		"CFNetworkExecuteProxyAutoConfigurationScript": &p.executeScript,
	} {
		purego.RegisterLibFunc(fn, nw, name)
	}
	for _, name := range []string{
		"kCFProxyTypeKey", "kCFProxyHostNameKey", "kCFProxyPortNumberKey", "kCFProxyTypeNone",
		"kCFProxyTypeHTTP", "kCFProxyTypeHTTPS", "kCFProxyTypeSOCKS", "kCFProxyAutoConfigurationURLKey",
		"kCFProxyAutoConfigurationJavaScriptKey", "kCFProxyTypeAutoConfigurationURL", "kCFProxyTypeAutoConfigurationJavaScript",
	} {
		address, err := purego.Dlsym(nw, name)
		if err != nil {
			return nil, err
		}
		p.keys[name] = *(*uintptr)(unsafe.Pointer(address))
	}
	p.callback = purego.NewCallback(func(id, list, nativeError uintptr) {
		if value, ok := pacCallbacks.Load(id); ok {
			result := value.(*cfPACResult)
			if list != 0 {
				result.list = p.retain(list)
			}
			result.failed, result.done = nativeError != 0, true
		}
	})
	return p, nil
})

const cfUTF8 = 0x08000100

type cfPACContext struct {
	version                            int
	info, retain, release, description uintptr
}

type cfPACResult struct {
	list         uintptr
	done, failed bool
}

var pacCallbacks sync.Map
var pacSequence atomic.Uintptr

func systemRoutes(ctx context.Context, target *url.URL) ([]*url.URL, error) {
	p, err := macProxy()
	if err != nil {
		return nil, err
	}
	settings := p.settings()
	if settings == 0 {
		return nil, nil
	}
	defer p.release(settings)
	value := p.makeString(0, target.String(), cfUTF8)
	if value == 0 {
		return nil, errors.New("invalid proxy lookup URL")
	}
	defer p.release(value)
	uri := p.makeURL(0, value, 0)
	if uri == 0 {
		return nil, errors.New("invalid proxy lookup URL")
	}
	defer p.release(uri)
	list := p.proxiesForURL(uri, settings)
	if list == 0 {
		return nil, errors.New("macOS proxy lookup failed")
	}
	defer p.release(list)
	values, err := p.routeValues(ctx, list, uri, false)
	if err != nil {
		return nil, err
	}
	return parseRoutes(values)
}

func (p *cfProxy) routeValues(ctx context.Context, list, uri uintptr, fromPAC bool) ([]string, error) {
	var routes []string
	var pacError error
	for i := range p.arrayCount(list) {
		entry := p.arrayGet(list, i)
		kind := p.dictGet(entry, p.keys["kCFProxyTypeKey"])
		if kind == 0 {
			routes = append(routes, "unsupported:")
			continue
		}
		is := func(key string) bool { return p.equal(kind, p.keys[key]) }
		scheme := "http"
		switch {
		case is("kCFProxyTypeNone"):
			routes = append(routes, "direct://")
			continue
		case is("kCFProxyTypeHTTP"), is("kCFProxyTypeHTTPS"):
			// CFNetwork labels an HTTP CONNECT proxy by the target scheme.

		case is("kCFProxyTypeSOCKS"):
			scheme = "socks5"
		case !fromPAC && (is("kCFProxyTypeAutoConfigurationURL") || is("kCFProxyTypeAutoConfigurationJavaScript")):
			execute, key := p.executeURL, "kCFProxyAutoConfigurationURLKey"
			if is("kCFProxyTypeAutoConfigurationJavaScript") {
				execute, key = p.executeScript, "kCFProxyAutoConfigurationJavaScriptKey"
			}
			result, err := p.resolvePAC(ctx, execute, p.dictGet(entry, p.keys[key]), uri)
			if err != nil {
				pacError = err
			} else {
				routes = append(routes, result...)
			}
			continue
		default:
			routes = append(routes, "unsupported:")
			continue
		}
		host := p.text(p.dictGet(entry, p.keys["kCFProxyHostNameKey"]))
		port := p.dictGet(entry, p.keys["kCFProxyPortNumberKey"])
		var number int32
		if host == "" || port == 0 || !p.numberGet(port, 3, &number) || number < 1 || number > 65535 {
			routes = append(routes, "unsupported:")
			continue
		}
		routes = append(routes, scheme+"://"+net.JoinHostPort(host, strconv.Itoa(int(number))))
	}
	if len(routes) == 0 && pacError != nil {
		return nil, pacError
	}
	return routes, nil
}

func (p *cfProxy) text(value uintptr) string {
	if value == 0 {
		return ""
	}
	b := make([]byte, 4*p.stringLen(value)+1)
	if !p.stringGet(value, b, len(b), cfUTF8) {
		return ""
	}
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return ""
}

func (p *cfProxy) resolvePAC(ctx context.Context, execute func(uintptr, uintptr, uintptr, *cfPACContext) uintptr, pac, uri uintptr) ([]string, error) {
	if pac == 0 {
		return nil, errors.New("missing macOS proxy auto-configuration")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	mode := p.makeString(0, "MicrolightProxyResolution", cfUTF8)
	defer p.release(mode)
	result := &cfPACResult{}
	id := pacSequence.Add(1)
	pacCallbacks.Store(id, result)
	defer pacCallbacks.Delete(id)
	source := execute(pac, uri, p.callback, &cfPACContext{info: id})
	if source == 0 {
		return nil, errors.New("macOS could not start PAC evaluation")
	}
	defer p.release(source)
	defer p.invalidateSource(source)
	p.addSource(p.runLoop(), source, mode)
	for !result.done {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if p.runInMode(mode, .1, true) == 1 && !result.done {
			return nil, errors.New("macOS PAC run loop ended")
		}
	}
	if result.list != 0 {
		defer p.release(result.list)
	}
	if result.failed || result.list == 0 {
		return nil, errors.New("macOS PAC evaluation failed")
	}
	return p.routeValues(ctx, result.list, uri, true)
}
