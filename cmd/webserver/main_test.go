package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSiteRoutes(t *testing.T) {
	site := fstest.MapFS{
		"index.html":       {Data: []byte("home")},
		"docs/index.html":  {Data: []byte("docs")},
		"empty/file.txt":   {Data: []byte("text")},
		"app.wasm":         {Data: []byte{0, 97, 115, 109}},
		"sqlite/worker.js": {Data: []byte("worker")},
	}
	handler := siteHandler(site)
	for _, tc := range []struct {
		method, url, accept string
		status              int
		body                string
	}{
		{"GET", "/", "", 200, "home"},
		{"GET", "/docs/", "", 200, "docs"},
		{"GET", "/docs", "", 301, ""},
		{"GET", "/chat/123", "text/html", 200, "home"},
		{"GET", "/chat/123", "*/*", 200, "home"},
		{"GET", "/chat/123?x=1", "text/html", 200, "home"},
		{"GET", "/app.wasm", "", 200, string([]byte{0, 97, 115, 109})},
		{"HEAD", "/app.wasm", "", 200, ""},
		{"GET", "/sqlite/missing.js", "text/html", 404, ""},
		{"GET", "/missing.png", "", 404, ""},
		{"GET", "/missing", "application/json", 404, ""},
		{"GET", "/empty/", "", 404, ""},
		{"GET", "/.keep", "", 404, ""},
		{"POST", "/", "", 405, ""},
	} {
		t.Run(tc.method+tc.url, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.url, nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			if tc.body != "" && w.Body.String() != tc.body {
				t.Fatalf("body %q", w.Body.String())
			}
			if tc.method == http.MethodHead && w.Body.Len() != 0 {
				t.Fatal("HEAD body")
			}
			if strings.HasSuffix(tc.url, ".wasm") && w.Header().Get("Content-Type") != "application/wasm" {
				t.Fatal("wrong WASM MIME type")
			}
		})
	}
}
