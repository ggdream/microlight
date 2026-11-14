package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const desktopPath = "/v1/desktop/"
const desktopSessionHeader = "Microlight-Desktop-Session"

type desktopRequest struct {
	X, Y *int
	Text string
}

type desktopInfo struct {
	SessionID    string
	Capabilities ComputerCapabilities
}

// DesktopHandler serves a single interactive desktop for the lifetime of
// sessionID. The process running it owns the native driver.
func DesktopHandler(driver ComputerDriver, sessionID, token string) http.Handler {
	gate := make(chan struct{}, 1)
	auth := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		provided := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if len(token) < 32 || subtle.ConstantTimeCompare(provided[:], auth[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		method, ok := strings.CutPrefix(r.URL.Path, desktopPath)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if method != "info" && r.Header.Get(desktopSessionHeader) != sessionID {
			http.Error(w, "desktop session changed", http.StatusConflict)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		case <-ctx.Done():
			http.Error(w, "desktop busy", http.StatusServiceUnavailable)
			return
		}
		var args desktopRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&args); err != nil || decoder.Decode(new(any)) != io.EOF ||
			(method == "click" && (args.X == nil || args.Y == nil)) {
			http.Error(w, "invalid desktop arguments", http.StatusBadRequest)
			return
		}
		caps, err := driver.GetCapabilities(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if method != "info" && !desktopOperationAvailable(caps, method) {
			http.Error(w, "desktop operation unavailable", http.StatusForbidden)
			return
		}
		if ctx.Err() != nil {
			http.Error(w, "desktop request expired", http.StatusRequestTimeout)
			return
		}
		var result any
		switch method {
		case "info":
			result = desktopInfo{SessionID: sessionID, Capabilities: caps}
		case "screenshot":
			result, err = driver.Screenshot(ctx)
		case "windows":
			result, err = driver.ListWindows(ctx)
		case "click":
			err = driver.Click(ctx, *args.X, *args.Y)
		case "text":
			err = driver.TypeText(ctx, args.Text)
		case "key":
			err = driver.PressKey(ctx, args.Text)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}

func desktopOperationAvailable(caps ComputerCapabilities, method string) bool {
	switch method {
	case "screenshot":
		return caps.Screenshot
	case "windows":
		return caps.ListWindows
	case "click":
		return caps.Click
	case "text":
		return caps.TypeText
	case "key":
		return caps.PressKey
	}
	return false
}

type remoteDesktop struct {
	client          *http.Client
	endpoint, token string
	info            desktopInfo
}

func connectDesktop(ctx context.Context, target Target, token string) (Runtime, error) {
	endpoint, err := url.Parse(target.Resource)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.ForceQuery ||
		endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return nil, errors.New("enter a desktop executor origin without credentials or a path")
	}
	host := endpoint.Hostname()
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && (net.ParseIP(host).IsLoopback() || strings.EqualFold(host, "localhost"))) {
		return nil, errors.New("use HTTPS for a remote desktop or HTTP through a loopback tunnel")
	}
	if token == "" && target.Credential != "" {
		token, err = keyring.Get(keyringService, target.Credential)
		if err != nil {
			return nil, err
		}
	}
	if len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("desktop executor requires a token of at least 32 characters")
	}
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		IdleConnTimeout:     30 * time.Second,
	}
	d := &remoteDesktop{
		client:   &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		endpoint: strings.TrimSuffix(endpoint.String(), "/"), token: token,
	}
	if err := d.call(ctx, "info", desktopRequest{}, &d.info); err != nil {
		d.Close()
		return nil, err
	}
	if d.info.SessionID == "" || d.info.Capabilities == (ComputerCapabilities{}) {
		d.Close()
		return nil, errors.New("remote interactive desktop unavailable")
	}
	return d, nil
}

func (d *remoteDesktop) call(ctx context.Context, method string, args desktopRequest, result any) error {
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint+desktopPath+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set(desktopSessionHeader, d.info.SessionID)
	resp, err := d.client.Do(req)
	if err != nil {
		if result == nil {
			return fmt.Errorf("desktop input outcome unknown: %w", err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("desktop executor returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	if result == nil {
		if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 64)); err != nil {
			return fmt.Errorf("desktop input outcome unknown: %w", err)
		}
		return nil
	}
	const limit = 48 << 20
	body, err = io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if len(body) > limit {
		return errors.New("desktop response exceeded size limit")
	}
	return json.Unmarshal(body, result)
}

func (d *remoteDesktop) Environment() Environment  { return Environment{} }
func (d *remoteDesktop) Close() error              { d.client.CloseIdleConnections(); return nil }
func (d *remoteDesktop) Computer() ComputerDriver  { return d }
func (d *remoteDesktop) ComputerSessionID() string { return d.info.SessionID }
func (d *remoteDesktop) GetCapabilities(ctx context.Context) (ComputerCapabilities, error) {
	return d.info.Capabilities, ctx.Err()
}
func (d *remoteDesktop) Screenshot(ctx context.Context) (image Image, err error) {
	err = d.call(ctx, "screenshot", desktopRequest{}, &image)
	return
}
func (d *remoteDesktop) ListWindows(ctx context.Context) (windows []WindowInfo, err error) {
	err = d.call(ctx, "windows", desktopRequest{}, &windows)
	return
}
func (d *remoteDesktop) Click(ctx context.Context, x, y int) error {
	return d.call(ctx, "click", desktopRequest{X: &x, Y: &y}, nil)
}
func (d *remoteDesktop) TypeText(ctx context.Context, text string) error {
	return d.call(ctx, "text", desktopRequest{Text: text}, nil)
}
func (d *remoteDesktop) PressKey(ctx context.Context, key string) error {
	return d.call(ctx, "key", desktopRequest{Text: key}, nil)
}
