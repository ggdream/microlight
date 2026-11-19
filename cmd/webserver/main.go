// Command webserver serves the embedded Microlight web application.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

// Populated by scripts/package-web.sh. The marker permits ordinary go test
// without committing generated application binaries.
//
//go:embed all:site
var content embed.FS

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()
	site, err := fs.Sub(content, "site")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := fs.Stat(site, "index.html"); err != nil {
		log.Fatal("web assets missing; build with bash scripts/package-web.sh")
	}
	server := &http.Server{Addr: *addr, Handler: siteHandler(site), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Microlight web listening on %s", listener.Addr())
	log.Fatal(server.Serve(listener))
}

func siteHandler(site fs.FS) http.Handler {
	files := http.FileServerFS(site)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "."
		}
		for _, part := range strings.Split(name, "/") {
			if strings.HasPrefix(part, ".") && part != "." {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		info, err := fs.Stat(site, name)
		if err == nil && info.IsDir() {
			index := path.Join(name, "index.html")
			if _, err = fs.Stat(site, index); err != nil {
				http.NotFound(w, r)
				return
			}
			files.ServeHTTP(w, r)
			return
		}
		if err == nil {
			files.ServeHTTP(w, r)
			return
		}
		// Only document navigation falls back. Missing assets remain real 404s.
		if path.Ext(name) != "" || (r.Header.Get("Accept") != "" && !strings.Contains(r.Header.Get("Accept"), "text/html") && !strings.Contains(r.Header.Get("Accept"), "*/*")) {
			http.NotFound(w, r)
			return
		}
		clone := r.Clone(r.Context())
		url := *r.URL
		url.Path = "/"
		url.RawPath = ""
		clone.URL = &url
		files.ServeHTTP(w, clone)
	})
}
