// Package testutil starts the demo server for tests.
package testutil

import (
	"net/http/httptest"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/demoserver"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// NewHTTPBin starts the demo server (an httpbin.org look-alike with SSE,
// GraphQL and WebSocket endpoints) on a random local port.
func NewHTTPBin() *httptest.Server {
	return httptest.NewServer(demoserver.Handler())
}

// UseLocal points the sample workspace's URL variables at a server from
// NewHTTPBin, so its requests (including the GraphQL, SSE and WebSocket
// samples) run without the internet.
func UseLocal(ws *models.Workspace, base string) {
	ws.SetVariable("baseUrl", base)
	ws.SetVariable("graphqlUrl", base+"/graphql")
	ws.SetVariable("sseUrl", base+"/sse?count=3&interval=10")
	ws.SetVariable("wsUrl", "ws"+strings.TrimPrefix(base, "http")+"/ws")
}
