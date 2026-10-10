package webui

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/pkg/wsclient"
)

// socketIn is a command from the page.
type socketIn struct {
	Type    string         `json:"type"` // connect, send, close
	Request models.Request `json:"request"`
	Message string         `json:"message"` // send: the Body tab, {{variables}} resolved here
}

// socket bridges the page to a WebSocket server. The page opens
// /api/socket, sends {"type":"connect","request":...}, then "send" and
// "close" commands; it gets connected, message, closed and error events.
func (s *Server) socket(w http.ResponseWriter, r *http.Request) {
	// Accept checks that the Origin is this page (no cross-site sockets).
	page, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer page.CloseNow()
	page.SetReadLimit(wsclient.MaxMessageSize)
	ctx := r.Context()

	var wmu sync.Mutex
	emit := func(v any) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = wsjson.Write(ctx, page, v)
	}

	var (
		conn     *wsclient.Conn
		cancel   context.CancelFunc = func() {}
		req      models.Request
		received []string
		lastBody string
		started  time.Time
		closedBy string
		mu       sync.Mutex
	)
	defer func() { cancel() }()

	for {
		var in socketIn
		if err := wsjson.Read(ctx, page, &in); err != nil {
			if conn != nil {
				conn.Close()
			}
			return
		}
		switch in.Type {
		case "connect":
			if conn != nil {
				emit(map[string]any{"type": "error", "error": "already connected"})
				continue
			}
			req = in.Request
			req.Type = models.TypeWebSocket
			s.mu.Lock()
			vars, settings, env := s.ws.VariableMap(), s.ws.Settings, s.ws.ActiveEnvironment
			s.mu.Unlock()
			if strings.TrimSpace(req.PreRequest) != "" {
				assignments, errs := script.RunPre(req.PreRequest, vars)
				s.apply(assignments)
				if len(errs) > 0 {
					emit(map[string]any{"type": "error", "error": "pre-request script: " + errs[0].Error()})
					continue
				}
			}
			p, err := engine.Prepare(req, vars)
			if err != nil {
				emit(map[string]any{"type": "error", "error": err.Error()})
				continue
			}
			tctx, tcancel := context.WithCancel(context.Background())
			cancel = tcancel // the deferred call ends the read loop when the page goes
			c, hs, err := wsclient.Dial(tctx, p.Request, wsclient.Options{
				Timeout:            time.Duration(settings.TimeoutSeconds) * time.Second,
				InsecureSkipVerify: settings.InsecureSkipVerify,
			})
			if err != nil {
				cancel()
				emit(map[string]any{"type": "error", "error": err.Error()})
				continue
			}
			conn, started = c, time.Now()
			emit(map[string]any{"type": "connected", "url": p.Request.URL.String(), "env": env, "missing": p.Missing,
				"requestHeaders": p.Request.Header, "status": hs.Status, "headers": hs.Headers, "protocol": hs.Protocol})
			go func() {
				for {
					m, err := c.Read(tctx)
					if err != nil {
						mu.Lock()
						how := closedBy
						if how == "" {
							how = wsclient.Describe(err)
						}
						events := append([]string(nil), received...)
						body := lastBody
						mu.Unlock()
						var tests []script.Result
						if strings.TrimSpace(req.Tests) != "" {
							s.mu.Lock()
							vars := s.ws.VariableMap()
							s.mu.Unlock()
							tests = script.RunTests(req.Tests, script.Response{
								Status: http.StatusSwitchingProtocols, Headers: hs.Headers, Body: []byte(body),
								Duration: time.Since(started), Events: events,
							}, vars)
							var caps []script.Assignment
							for _, t := range tests {
								if t.Capture != nil {
									caps = append(caps, *t.Capture)
								}
							}
							s.apply(caps)
						}
						emit(map[string]any{"type": "closed", "how": how, "durationMs": time.Since(started).Milliseconds(), "tests": testsJSON(tests)})
						return
					}
					mu.Lock()
					received = append(received, m.Data)
					lastBody = m.Data
					mu.Unlock()
					emit(map[string]any{"type": "message", "dir": "in", "data": m.Data, "binary": m.Binary, "size": m.Size, "at": m.At.UnixMilli()})
				}
			}()
		case "send":
			if conn == nil {
				emit(map[string]any{"type": "error", "error": "not connected"})
				continue
			}
			msgReq := req
			msgReq.Body = in.Message
			s.mu.Lock()
			vars := s.ws.VariableMap()
			s.mu.Unlock()
			p, err := engine.Prepare(msgReq, vars)
			if err != nil {
				emit(map[string]any{"type": "error", "error": err.Error()})
				continue
			}
			m, err := conn.Send(ctx, p.Message)
			if err != nil {
				emit(map[string]any{"type": "error", "error": "send failed: " + err.Error()})
				continue
			}
			emit(map[string]any{"type": "message", "dir": "out", "data": m.Data, "size": m.Size, "at": m.At.UnixMilli()})
		case "close":
			if conn != nil {
				mu.Lock()
				closedBy = "disconnected"
				mu.Unlock()
				conn.Close()
				cancel()
			}
		}
	}
}
