package proxy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winHTTP               = windows.NewLazySystemDLL("winhttp.dll")
	openWinHTTP           = winHTTP.NewProc("WinHttpOpen")
	closeWinHTTP          = winHTTP.NewProc("WinHttpCloseHandle")
	currentWinHTTPProxy   = winHTTP.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	setWinHTTPCallback    = winHTTP.NewProc("WinHttpSetStatusCallback")
	createWinHTTPResolver = winHTTP.NewProc("WinHttpCreateProxyResolver")
	resolveWinHTTPProxy   = winHTTP.NewProc("WinHttpGetProxyForUrlEx")
	getWinHTTPResult      = winHTTP.NewProc("WinHttpGetProxyResult")
	freeWinHTTPResult     = winHTTP.NewProc("WinHttpFreeProxyResult")
	globalFree            = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")

	proxyCallbacks sync.Map
	proxySequence  atomic.Uintptr
	proxyCallback  = windows.NewCallback(winHTTPStatus)
)

const (
	winProxyComplete          = 0x01000000
	winRequestError           = 0x00200000
	winAsync                  = 0x10000000
	winNoProxy                = 1
	winAutoDetect             = 1
	winPACURL                 = 2
	winAllowStatic            = 0x200
	winAllowConnectionManager = 0x400

	// WinHTTP defines INTERNET_SCHEME values separately from WinINet.
	winSchemeHTTP  = 1
	winSchemeHTTPS = 2
	winSchemeSOCKS = 4
)

type winProxyConfig struct {
	autoDetect         int32
	pac, proxy, bypass *uint16
}

type winProxyOptions struct {
	flags, detect uint32
	pac           *uint16
	reserved      uintptr
	reservedFlags uint32
	autoLogon     int32
}

type winProxyEntry struct {
	proxy, bypass, scheme int32
	host                  *uint16
	port                  uint16
}

type winProxyResult struct {
	count   uint32
	entries *winProxyEntry
}

type winAsyncResult struct {
	result uintptr
	code   uint32
}

func winHTTPStatus(_ uintptr, id uintptr, status uint32, info *winAsyncResult, _ uint32) uintptr {
	value, ok := proxyCallbacks.LoadAndDelete(id)
	if !ok {
		return 0
	}
	code := windows.Errno(0)
	if status == winRequestError {
		code = windows.ERROR_GEN_FAILURE
		if info != nil {
			code = windows.Errno(info.code)
		}
	}
	value.(chan windows.Errno) <- code
	return 0
}

func systemRoutes(ctx context.Context, target *url.URL) ([]*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var settings winProxyConfig
	ok, _, callErr := currentWinHTTPProxy.Call(uintptr(unsafe.Pointer(&settings)))
	if ok == 0 {
		return nil, callErr
	}
	for _, pointer := range []*uint16{settings.pac, settings.proxy, settings.bypass} {
		if pointer != nil {
			defer globalFree.Call(uintptr(unsafe.Pointer(pointer)))
		}
	}
	if settings.autoDetect == 0 && settings.pac == nil && settings.proxy == nil {
		return nil, nil
	}

	session, _, callErr := openWinHTTP.Call(0, winNoProxy, 0, 0, winAsync)
	if session == 0 {
		return nil, callErr
	}
	defer closeWinHTTP.Call(session)
	old, _, callErr := setWinHTTPCallback.Call(session, proxyCallback, winProxyComplete|winRequestError, 0)
	if old == ^uintptr(0) {
		return nil, callErr
	}
	var handle uintptr
	code, _, _ := createWinHTTPResolver.Call(session, uintptr(unsafe.Pointer(&handle)))
	if code != 0 {
		return nil, windows.Errno(code)
	}
	defer closeWinHTTP.Call(handle)

	options := winProxyOptions{flags: winAllowStatic | winAllowConnectionManager, pac: settings.pac}
	if settings.pac != nil {
		options.flags |= winPACURL
	}
	if settings.autoDetect != 0 {
		options.flags |= winAutoDetect
		options.detect = 3 // DHCP and DNS.
	}
	uri, err := windows.UTF16PtrFromString(target.String())
	if err != nil {
		return nil, err
	}
	id := proxySequence.Add(1)
	done := make(chan windows.Errno, 1)
	proxyCallbacks.Store(id, done)
	defer proxyCallbacks.Delete(id)
	code, _, _ = resolveWinHTTPProxy.Call(handle, uintptr(unsafe.Pointer(uri)), uintptr(unsafe.Pointer(&options)), id)
	if windows.Errno(code) != windows.ERROR_IO_PENDING {
		return nil, fmt.Errorf("start WinHTTP proxy lookup: %w", windows.Errno(code))
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		if result != 0 {
			// Windows enables discovery by default even on networks with no WPAD.
			const autoDetectionFailed = 12180
			if result == autoDetectionFailed && settings.pac == nil && settings.proxy == nil {
				return nil, nil
			}
			return nil, result
		}
	}

	var result winProxyResult
	code, _, _ = getWinHTTPResult.Call(handle, uintptr(unsafe.Pointer(&result)))
	if code != 0 {
		return nil, windows.Errno(code)
	}
	defer freeWinHTTPResult.Call(uintptr(unsafe.Pointer(&result)))
	values := make([]string, 0, result.count)
	for _, entry := range unsafe.Slice(result.entries, int(result.count)) {
		if entry.proxy == 0 {
			values = append(values, "direct://")
			continue
		}
		scheme := ""
		switch entry.scheme {
		case winSchemeHTTP:
			scheme = "http"
		case winSchemeHTTPS:
			scheme = "https"
		case winSchemeSOCKS:
			scheme = "socks5"
		}
		if scheme == "" || entry.host == nil {
			values = append(values, "unsupported:")
			continue
		}
		host := windows.UTF16PtrToString(entry.host)
		if entry.port != 0 {
			host = net.JoinHostPort(host, strconv.Itoa(int(entry.port)))
		}
		values = append(values, scheme+"://"+host)
	}
	return parseRoutes(values)
}
