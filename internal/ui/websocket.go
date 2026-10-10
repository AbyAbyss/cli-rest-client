package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/pkg/wsclient"
)

// wsSession is a WebSocket connection and what happened on it.
type wsSession struct {
	conn      *wsclient.Conn
	cancel    context.CancelFunc
	handshake *wsclient.Handshake
	started   time.Time
	ended     time.Time // zero while open
	how       string    // how it ended
	messages  []wsclient.Message
	tests     string // the request's tests, run when the connection ends
}

func (s *wsSession) open() bool { return s != nil && s.conn != nil && s.ended.IsZero() }

func (s *wsSession) counts() (sent, received int) {
	for _, m := range s.messages {
		if m.Sent {
			sent++
		} else {
			received++
		}
	}
	return
}

// wsSend connects, or sends the Body tab's message when connected.
func (a *App) wsSend() {
	if s := a.wsSession(); s.open() {
		a.wsSendMessage(s)
		return
	}
	if a.sending {
		a.setStatus(levelWarning, "Already connecting. Press Esc to cancel.")
		return
	}
	req := a.req.Clone()
	variables := a.ws.VariableMap()
	if strings.TrimSpace(req.PreRequest) != "" {
		assignments, errs := script.RunPre(req.PreRequest, variables)
		a.applyAssignments(assignments)
		if len(errs) > 0 {
			a.result = &sendResult{method: "WS", url: req.URL, env: a.ws.ActiveEnvironment, err: fmt.Errorf("pre-request script: %v", errs[0])}
			a.renderResponse()
			a.setStatus(levelError, "Pre-request script failed")
			return
		}
	}
	p, err := engine.Prepare(req, variables)
	if err != nil {
		a.result = &sendResult{method: "WS", url: req.URL, env: a.ws.ActiveEnvironment, err: err}
		a.renderResponse()
		a.setStatus(levelError, err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess := &wsSession{cancel: cancel, started: time.Now(), tests: req.Tests}
	res := &sendResult{
		method:     "WS",
		url:        p.Request.URL.String(),
		started:    sess.started,
		missing:    p.Missing,
		reqHeaders: p.Request.Header.Clone(),
		request:    req,
		source:     a.linkedPath(),
		env:        a.ws.ActiveEnvironment,
		socket:     sess,
	}
	a.result = res
	a.sending, a.cancel = true, cancel
	a.renderResponse()
	a.renderTitles()
	a.setStatus(levelInfo, "Connecting to "+res.url)

	opts := wsclient.Options{
		Timeout:            time.Duration(a.ws.Settings.TimeoutSeconds) * time.Second,
		InsecureSkipVerify: a.ws.Settings.InsecureSkipVerify,
	}
	go func() {
		conn, hs, err := wsclient.Dial(ctx, p.Request, opts)
		a.tv.QueueUpdateDraw(func() {
			a.sending, a.cancel = false, nil
			if err != nil {
				cancelled := ctx.Err() != nil
				cancel()
				if cancelled {
					res.cancelled = true
					a.setStatus(levelWarning, "Connection cancelled")
				} else {
					res.err = err
					a.setStatus(levelError, "Could not connect")
				}
				res.socket = nil
				a.renderResponse()
				a.renderTitles()
				return
			}
			sess.conn, sess.handshake = conn, hs
			sess.started = time.Now()
			a.setStatus(levelSuccess, "Connected. Ctrl+R sends the message in the Body tab, Esc disconnects")
			a.renderResponse()
			a.renderTitles()
			go a.wsReadLoop(ctx, res, sess)
		})
	}()
}

// wsSession is the session shown in the response pane, if any.
func (a *App) wsSession() *wsSession {
	if a.result == nil {
		return nil
	}
	return a.result.socket
}

func (a *App) wsSendMessage(s *wsSession) {
	p, err := engine.Prepare(a.req, a.ws.VariableMap())
	if err != nil {
		a.setStatus(levelError, err.Error())
		return
	}
	if p.Message == "" {
		a.setStatus(levelWarning, "Write a message in the Body tab first")
		return
	}
	m, err := s.conn.Send(context.Background(), p.Message)
	if err != nil {
		a.setStatus(levelError, "Send failed: "+err.Error())
		return
	}
	s.messages = append(s.messages, m)
	a.setStatus(levelSuccess, fmt.Sprintf("Sent %s", humanBytes(m.Size)))
	a.renderResponse()
	a.response.ScrollToEnd()
}

func (a *App) wsReadLoop(ctx context.Context, res *sendResult, s *wsSession) {
	for {
		m, err := s.conn.Read(ctx)
		if err != nil {
			how := wsclient.Describe(err)
			a.tv.QueueUpdateDraw(func() { a.wsEnded(res, s, how) })
			return
		}
		a.tv.QueueUpdateDraw(func() {
			s.messages = append(s.messages, m)
			if res == a.result {
				a.renderResponse()
				if a.tv.GetFocus() != a.response {
					a.response.ScrollToEnd()
				}
			}
		})
	}
}

// wsDisconnect closes the connection; wsEnded finishes up when the read
// loop notices.
func (a *App) wsDisconnect() {
	s := a.wsSession()
	if !s.open() {
		return
	}
	s.how = "disconnected"
	_ = s.conn.Close()
	s.cancel()
}

// wsEnded records how a connection ended and runs the tests on the
// messages it received.
func (a *App) wsEnded(res *sendResult, s *wsSession, how string) {
	if !s.ended.IsZero() {
		return
	}
	s.ended = time.Now()
	if s.how == "" {
		s.how = how
	}
	s.cancel()
	var events []string
	var last []byte
	for _, m := range s.messages {
		if !m.Sent {
			events = append(events, m.Data)
			last = []byte(m.Data)
		}
	}
	if strings.TrimSpace(s.tests) != "" {
		h := http.Header{}
		if s.handshake != nil {
			h = s.handshake.Headers
		}
		res.tests = script.RunTests(s.tests, script.Response{
			Status: http.StatusSwitchingProtocols, Headers: h, Body: last, Duration: s.ended.Sub(s.started), Events: events,
		}, a.ws.VariableMap())
		var captures []script.Assignment
		for _, r := range res.tests {
			if r.Capture != nil {
				captures = append(captures, *r.Capture)
			}
		}
		a.applyAssignments(captures)
	}
	sent, received := s.counts()
	msg := fmt.Sprintf("WebSocket %s after %s: %d sent, %d received", s.how, streamDuration(s.ended.Sub(s.started)), sent, received)
	level := levelInfo
	if passed, total := testCounts(res.tests); total > 0 {
		msg += fmt.Sprintf(", tests %d/%d passed", passed, total)
		if passed < total {
			level = levelWarning
		}
	}
	if res == a.result {
		a.setStatus(level, msg)
		a.renderResponse()
		a.renderTitles()
	}
}

// renderWebSocket draws a WebSocket session in the response pane.
func (a *App) renderWebSocket(res *sendResult, line func(string, ...any), section func(id, title, extra string) bool) {
	t := a.theme
	s := res.socket
	sent, received := s.counts()
	switch {
	case s.conn == nil:
		line("[%s]Connecting…  [%s](Esc to cancel)", t.HexWarning, t.HexMuted)
		a.response.SetTitle(" WebSocket ")
		return
	case s.open():
		line("[%s::b]● Connected[-:-:-] [%s]%s · ↑ %d sent · ↓ %d received  [%s](Ctrl+R sends the Body tab, Esc disconnects)",
			t.HexSuccess, t.HexText, streamDuration(time.Since(s.started)), sent, received, t.HexMuted)
		a.response.SetTitle(fmt.Sprintf(" WebSocket · [%s]open[-] ", t.HexSuccess))
	default:
		line("[%s]■ %s [%s]after %s · ↑ %d sent · ↓ %d received", t.HexInfo, tview.Escape(capitalize(s.how)),
			t.HexText, streamDuration(s.ended.Sub(s.started)), sent, received)
		a.response.SetTitle(" WebSocket · closed ")
	}

	if len(res.tests) > 0 {
		a.writeTests(res, line, section)
	}
	if section(secReqHeaders, "Handshake", fmt.Sprintf("[%s]%d %s", t.HexMuted, s.handshake.Status, http.StatusText(s.handshake.Status))) {
		writeHeaders(line, res.reqHeaders, t)
		line("  [%s]──", t.HexMuted)
		writeHeaders(line, s.handshake.Headers, t)
		if s.handshake.Protocol != "" {
			line("  [%s]Subprotocol: [%s]%s", t.HexMuted, t.HexText, tview.Escape(s.handshake.Protocol))
		}
	}
	if section(secBody, "Messages", fmt.Sprintf("[%s](%d)", t.HexMuted, len(s.messages))) {
		if len(s.messages) == 0 {
			line("  [%s]No messages yet.", t.HexMuted)
		}
		for _, m := range s.messages {
			arrow, color := "↓", t.HexSuccess
			if m.Sent {
				arrow, color = "↑", t.HexAccent
			}
			data := tview.Escape(m.Data)
			var compact bytes.Buffer
			if !m.Binary && json.Compact(&compact, []byte(m.Data)) == nil {
				data = highlightJSON(compact.String(), t)
			}
			kind := ""
			if m.Binary {
				kind = fmt.Sprintf(" [%s]binary", t.HexMuted)
			}
			line("  [%s::b]%s[-:-:-] [%s]%s%s  [%s]%s", color, arrow, t.HexMuted, m.At.Format("15:04:05.000"), kind,
				t.HexText, strings.ReplaceAll(data, "\n", "\n      "))
		}
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// setRequestType switches the builder between HTTP methods and WebSocket,
// from the method dropdown.
func (a *App) setRequestType(label string) {
	if label == "WS" {
		a.req.Type = models.TypeWebSocket
	} else {
		a.req.Type = models.TypeHTTP
		a.req.Method = label
	}
}
