// Package wsclient opens WebSocket connections for the app: a handshake
// built from a prepared HTTP request, text messages out, and every message
// in with its arrival time.
package wsclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
)

// MaxMessageSize caps a single incoming message.
const MaxMessageSize = 16 << 20

// Options configures Dial.
type Options struct {
	// Timeout for the handshake. Zero means 30 seconds.
	Timeout time.Duration
	// InsecureSkipVerify disables TLS certificate verification for wss://.
	InsecureSkipVerify bool
}

// Handshake is the server's answer to the upgrade request.
type Handshake struct {
	Status   int
	Proto    string
	Headers  http.Header
	Protocol string // the negotiated subprotocol, if any
}

// Message is one message on the connection.
type Message struct {
	At     time.Time
	Sent   bool // false for received messages
	Data   string
	Binary bool // a binary message; Data holds it as is (or a summary if not UTF-8)
	Size   int
}

// Conn is an open WebSocket connection.
type Conn struct {
	c *websocket.Conn
}

// Dial opens a connection with the URL and headers of req, a GET to a ws://
// or wss:// URL as engine.Prepare builds it. A Sec-WebSocket-Protocol header
// is used to negotiate subprotocols.
func Dial(ctx context.Context, req *http.Request, opts Options) (*Conn, *Handshake, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user opted in
	}
	header := req.Header.Clone()
	var protocols []string
	for _, p := range strings.Split(header.Get("Sec-WebSocket-Protocol"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			protocols = append(protocols, p)
		}
	}
	// The library writes the upgrade headers itself.
	for _, h := range []string{"Host", "Connection", "Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"} {
		header.Del(h)
	}

	c, resp, err := websocket.Dial(ctx, req.URL.String(), &websocket.DialOptions{
		HTTPClient:   &http.Client{Transport: transport},
		HTTPHeader:   header,
		Host:         req.Host,
		Subprotocols: protocols,
	})
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, nil, fmt.Errorf("the server refused the upgrade with %s", resp.Status)
		}
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == context.DeadlineExceeded {
			return nil, nil, fmt.Errorf("no handshake within %s", opts.Timeout)
		}
		return nil, nil, err
	}
	c.SetReadLimit(MaxMessageSize)
	hs := &Handshake{Status: resp.StatusCode, Proto: resp.Proto, Headers: resp.Header, Protocol: c.Subprotocol()}
	return &Conn{c: c}, hs, nil
}

// Send writes a text message.
func (c *Conn) Send(ctx context.Context, text string) (Message, error) {
	m := Message{At: time.Now(), Sent: true, Data: text, Size: len(text)}
	return m, c.c.Write(ctx, websocket.MessageText, []byte(text))
}

// Read waits for the next message. When the connection ends it returns an
// error describing how (see Describe).
func (c *Conn) Read(ctx context.Context) (Message, error) {
	typ, data, err := c.c.Read(ctx)
	if err != nil {
		return Message{}, err
	}
	m := Message{At: time.Now(), Data: string(data), Size: len(data)}
	if typ == websocket.MessageBinary {
		m.Binary = true
		if !utf8.Valid(data) {
			m.Data = fmt.Sprintf("(%d bytes of binary data)", len(data))
		}
	}
	return m, nil
}

// Close ends the connection with a normal closure.
func (c *Conn) Close() error {
	return c.c.Close(websocket.StatusNormalClosure, "")
}

// Describe explains why a connection ended, from the error Read returned.
func Describe(err error) string {
	if err == nil {
		return "closed"
	}
	if code := websocket.CloseStatus(err); code != -1 {
		var ce websocket.CloseError
		reason := ""
		if errors.As(err, &ce) && ce.Reason != "" {
			reason = ": " + ce.Reason
		}
		if code == websocket.StatusNormalClosure {
			return fmt.Sprintf("closed normally (%d)%s", code, reason)
		}
		return fmt.Sprintf("closed with %d %s%s", code, strings.TrimPrefix(code.String(), "Status"), reason)
	}
	if errors.Is(err, context.Canceled) {
		return "disconnected"
	}
	return "connection lost: " + err.Error()
}
