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
	server := &http.Server{
		Addr: *addr, Handler: workspace.DesktopHandler(driver, uuid.NewV7().String(), token),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second,
	}
	log.Printf("Desktop executor listening on %s", *addr)
	log.Fatal(server.ListenAndServe())
}
