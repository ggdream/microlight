package linkpreview

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMetadataAndRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/go", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/page", http.StatusFound)
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		if r.Referer() != "" || r.Header.Get("Cookie") != "" {
			t.Error("anonymous preview sent a referrer or cookie")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<title>Fallback</title><meta name="description" content="Fallback description">
<meta property="og:title" content="  示例 &amp; guide ">
<meta content="The page description" property="og:description">
<body><meta property="og:title" content="Ignored"></body>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	var paths []string
	check := func(_ context.Context, u *url.URL) error {
		paths = append(paths, u.Path)
		return nil
	}
	metadata, err := fetch(t.Context(), srv.URL+"/go#part", srv.Client().Transport, check)
	if err != nil || metadata != (Metadata{"示例 & guide", "The page description"}) {
		t.Fatalf("metadata = %+v, %v", metadata, err)
	}
	if strings.Join(paths, ",") != "/go,/page" {
		t.Fatalf("did not check both destinations: %v", paths)
	}
}

func TestPreviewRejectsLocalTargets(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/", "http://service.local/", "http://127.0.0.1/",
		"http://169.254.169.254/", "http://10.0.0.1/", "http://[::ffff:127.0.0.1]/",
		"http://[64:ff9b::7f00:1]/", "http://user:password@example.com/", "file:///index.html",
	} {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if checkPublic(t.Context(), u) == nil {
				t.Fatal("unexpected allowed target")
			}
		})
	}
}
