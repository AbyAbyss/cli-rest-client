package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/demoserver"
)

// Demo serves the demo server (the sample workspace's Local environment
// points at it) until ctx is cancelled.
func Demo(ctx context.Context, out, errOut io.Writer, args []string) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(errOut)
	addr := fs.String("addr", "localhost:8080", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			fmt.Fprintf(errOut, "demo: %s is already in use. Pick another address with -addr localhost:9090,\n"+
				"and point the Local environment's baseUrl at it.\n", *addr)
		} else {
			fmt.Fprintln(errOut, "demo:", err)
		}
		return 1
	}
	base := "http://" + l.Addr().String()
	if host, port, err := net.SplitHostPort(l.Addr().String()); err == nil && (host == "127.0.0.1" || host == "::1") && strings.HasPrefix(*addr, "localhost") {
		base = "http://localhost:" + port
	}
	fmt.Fprintf(out, `Demo server on %s (Ctrl+C stops it)

It answers every request in the sample workspace, including the GraphQL,
Server-Sent Events and WebSocket samples. In another terminal:

  term-rest-client -data demo.json        # the app, with a fresh sample workspace
  term-rest-client -data demo.json run -env Local "Auth API" "User Service/Lookup" "Payment Gateway"

-data demo.json keeps the samples in their own file, away from your workspace.
`, base)

	srv := &http.Server{Handler: demoserver.Handler(), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	select {
	case err := <-done:
		fmt.Fprintln(errOut, "demo:", err)
		return 1
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		fmt.Fprintln(out, "Demo server stopped.")
		return 0
	}
}
