package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"
	"uuid"

	"github.com/ggdream/microlight/internal/workspace"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:9420", "loopback address for a local client, SSH tunnel or TLS reverse proxy")
	root := flag.String("workspace", "", "directory to expose to authenticated workspace file tools")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("listen address must use a loopback IP")
	}
	token := os.Getenv("MICROLIGHT_DESKTOP_TOKEN")
	if len(token) < 32 {
		log.Fatal("set MICROLIGHT_DESKTOP_TOKEN to a secret of at least 32 characters")
	}
	driver := workspace.NewLocalComputer()
	if driver == nil {
		log.Fatal("native desktop execution is unavailable on this platform")
	}
	sessionID := uuid.NewV7().String()
	handler := workspace.DesktopHandler(driver, sessionID, token)
	if *root != "" {
		handler, err = workspace.DesktopWorkspaceHandler(driver, sessionID, token, *root)
		if err != nil {
			log.Fatal(err)
		}
	}
	server := &http.Server{
		Addr: *addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second,
	}
	log.Printf("Desktop executor listening on %s", *addr)
	log.Fatal(server.ListenAndServe())
}
