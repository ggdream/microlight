package proxy

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/net/http/httpproxy"
)

type gioProxy struct {
	defaultResolver func() uintptr
	typeName        func(uintptr) string
	lookup          func(uintptr, string, uintptr, *uintptr) **byte
	newCancellable  func() uintptr
	cancel          func(uintptr)
	unref           func(uintptr)
	freeVector      func(**byte)
	freeError       func(uintptr)
}

var linuxGIO = sync.OnceValue(func() *gioProxy {
	handle, err := purego.Dlopen("libgio-2.0.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	gio := &gioProxy{}
	purego.RegisterLibFunc(&gio.defaultResolver, handle, "g_proxy_resolver_get_default")
	purego.RegisterLibFunc(&gio.typeName, handle, "g_type_name_from_instance")
	purego.RegisterLibFunc(&gio.lookup, handle, "g_proxy_resolver_lookup")
	purego.RegisterLibFunc(&gio.newCancellable, handle, "g_cancellable_new")
	purego.RegisterLibFunc(&gio.cancel, handle, "g_cancellable_cancel")
	purego.RegisterLibFunc(&gio.unref, handle, "g_object_unref")
	purego.RegisterLibFunc(&gio.freeVector, handle, "g_strfreev")
	purego.RegisterLibFunc(&gio.freeError, handle, "g_error_free")
	return gio
})

func systemRoutes(ctx context.Context, target *url.URL) ([]*url.URL, error) {
	if gio := linuxGIO(); gio != nil {
		resolver := gio.defaultResolver()
		if resolver != 0 && gio.typeName(resolver) != "GDummyProxyResolver" {
			return gio.routes(ctx, resolver, target)
		}
	}
	// Non-desktop sessions commonly configure proxies through the environment.
	config := httpproxy.FromEnvironment()
	all := os.Getenv("all_proxy")
	if all == "" {
		all = os.Getenv("ALL_PROXY")
	}
	if config.HTTPProxy == "" {
		config.HTTPProxy = all
	}
	if config.HTTPSProxy == "" {
		config.HTTPSProxy = all
	}
	route, err := config.ProxyFunc()(target)
	if err != nil || route == nil {
		return nil, err
	}
	return parseRoutes([]string{route.String()})
}

func (gio *gioProxy) routes(ctx context.Context, resolver uintptr, target *url.URL) ([]*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cancellable := gio.newCancellable()
	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		gio.cancel(cancellable)
		close(finished)
	})
	defer func() {
		if !stop() {
			<-finished
		}
		gio.unref(cancellable)
	}()
	var nativeError uintptr
	values := gio.lookup(resolver, target.String(), cancellable, &nativeError)
	if nativeError != 0 {
		gio.freeError(nativeError)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("desktop proxy lookup failed")
	}
	if values == nil {
		return nil, errors.New("desktop proxy lookup returned no routes")
	}
	defer gio.freeVector(values)
	var routes []string
	for offset := uintptr(0); ; offset += unsafe.Sizeof(values) {
		value := *(**byte)(unsafe.Add(unsafe.Pointer(values), offset))
		if value == nil {
			break
		}
		n := 0
		for *(*byte)(unsafe.Add(unsafe.Pointer(value), n)) != 0 {
			n++
		}
		route := strings.Clone(unsafe.String(value, n))
		if strings.HasPrefix(route, "socks://") {
			route = "socks5://" + strings.TrimPrefix(route, "socks://")
		}
		routes = append(routes, route)
	}
	return parseRoutes(routes)
}
