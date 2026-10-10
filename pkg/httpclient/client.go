// Package httpclient sends HTTP requests and captures timing and body data.
// Streaming responses (Server-Sent Events, NDJSON) can be followed as they
// arrive with DoStream.
package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"
)

// MaxBodySize caps how much of a response body is read into memory.
const MaxBodySize = 50 << 20 // 50 MiB

// MaxEvents caps how many stream events are kept.
const MaxEvents = 10000

// Options configures a Client.
type Options struct {
	// Timeout for the whole exchange. Zero means 30 seconds. With DoStream it
	// only covers the wait for the response headers when the response turns
	// out to be a stream.
	Timeout time.Duration
	// DisableRedirects returns 3xx responses instead of following them.
	DisableRedirects bool
	// InsecureSkipVerify disables TLS certificate verification.
	InsecureSkipVerify bool
}

// Client wraps an http.Client.
type Client struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient creates a client from opts.
func NewClient(opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user opted in
	}
	// The timeout is applied per request in do, so a stream can outlive it.
	c := &http.Client{Transport: transport}
	if opts.DisableRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &Client{httpClient: c, timeout: opts.Timeout}
}

// Event is one Server-Sent Event, or one line of an NDJSON stream (then
// only At and Data are set).
type Event struct {
	At   time.Duration `json:"at"` // since the request started
	ID   string        `json:"id,omitempty"`
	Type string        `json:"type,omitempty"` // the SSE "event" field; empty means "message"
	Data string        `json:"data"`
}

// Response is an HTTP response with its body read.
type Response struct {
	StatusCode int
	Status     string // e.g. "200 OK"
	Proto      string
	Headers    http.Header
	Body       []byte
	// Truncated is true when the body was larger than MaxBodySize.
	Truncated bool
	Duration  time.Duration
	// URL is the final URL after redirects.
	URL string

	// Stream is the kind of streaming body: "sse", "ndjson" or "" for an
	// ordinary response.
	Stream string
	// Events are the parsed SSE events or NDJSON lines, oldest first.
	Events []Event
	// Done is false while a stream is still arriving (in DoStream updates).
	Done bool
	// Stopped is true when the caller ended a stream early; the response
	// holds what had arrived by then.
	Stopped bool
}

// StatusText returns the reason phrase without the numeric code ("OK").
func (r *Response) StatusText() string {
	prefix := fmt.Sprintf("%d ", r.StatusCode)
	if len(r.Status) > len(prefix) && r.Status[:len(prefix)] == prefix {
		return r.Status[len(prefix):]
	}
	if t := http.StatusText(r.StatusCode); t != "" {
		return t
	}
	return r.Status
}

// StreamKind reports which streaming format a Content-Type announces.
func StreamKind(contentType string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	switch mt {
	case "text/event-stream":
		return "sse"
	case "application/x-ndjson", "application/ndjson", "application/jsonl", "application/x-jsonlines",
		"application/jsonlines", "application/stream+json", "application/x-json-stream":
		return "ndjson"
	}
	return ""
}

// Do sends req and reads the whole response body, within the timeout.
func (c *Client) Do(ctx context.Context, req *http.Request) (*Response, error) {
	return c.do(ctx, req, nil)
}

// DoStream sends req like Do, but when the response is a stream (SSE or
// NDJSON) the timeout stops applying once the headers arrive, and update is
// called with a snapshot each time more of the body comes in. Cancelling ctx
// after the headers arrived ends the stream and returns what was received,
// with Stopped set. Snapshots share no mutable state with the client, so
// they can be handed to another goroutine.
func (c *Client) DoStream(ctx context.Context, req *http.Request, update func(*Response)) (*Response, error) {
	if update == nil {
		update = func(*Response) {}
	}
	return c.do(ctx, req, update)
}

func (c *Client) do(ctx context.Context, req *http.Request, update func(*Response)) (*Response, error) {
	start := time.Now()
	userCtx := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	timedOut := false
	timer := time.AfterFunc(c.timeout, func() {
		mu.Lock()
		timedOut = true
		mu.Unlock()
		cancel()
	})
	defer timer.Stop()
	fail := func(err error) error {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case timedOut:
			return fmt.Errorf("timed out after %s", c.timeout)
		case userCtx.Err() != nil:
			return context.Canceled
		}
		return err
	}

	resp, err := c.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	defer resp.Body.Close()

	out := &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Proto:      resp.Proto,
		Headers:    resp.Header,
		URL:        resp.Request.URL.String(),
		Stream:     StreamKind(resp.Header.Get("Content-Type")),
	}
	streaming := update != nil && out.Stream != ""
	if streaming {
		timer.Stop() // a stream may run as long as the server keeps it open
		update(out.snapshot(time.Since(start)))
	}

	var parser *streamParser
	if out.Stream != "" {
		parser = &streamParser{kind: out.Stream}
	}
	buf := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			now := time.Since(start)
			room := MaxBodySize - len(out.Body)
			if n > room {
				out.Truncated = true
				n = max(room, 0)
			}
			out.Body = append(out.Body, buf[:n]...)
			if parser != nil && len(out.Events) < MaxEvents {
				out.Events = parser.feed(buf[:n], now, out.Events)
			}
			if streaming {
				update(out.snapshot(now))
			}
			if out.Truncated {
				break
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if streaming && userCtx.Err() != nil {
				out.Stopped = true // the user ended the stream; keep what arrived
				break
			}
			if err := fail(rerr); errors.Is(err, context.Canceled) {
				return nil, err
			} else {
				return nil, fmt.Errorf("reading response body: %w", err)
			}
		}
	}
	if parser != nil && len(out.Events) < MaxEvents {
		out.Events = parser.flush(time.Since(start), out.Events)
	}
	out.Duration = time.Since(start)
	out.Done = true
	return out, nil
}

// snapshot is what the caller may read while the stream continues. Body
// and Events are only ever appended to, so capping the slices at their
// current length is enough: later appends never touch what they cover.
func (r *Response) snapshot(at time.Duration) *Response {
	s := *r
	s.Body = r.Body[:len(r.Body):len(r.Body)]
	s.Events = r.Events[:len(r.Events):len(r.Events)]
	s.Duration = at
	return &s
}

// streamParser turns body bytes into SSE events or NDJSON lines as they
// arrive, following the SSE parsing rules of the HTML standard.
type streamParser struct {
	kind    string
	partial []byte // an incomplete line
	skipLF  bool   // the last chunk ended in \r, so a leading \n is part of it

	data        []byte
	hasData     bool
	eventType   string
	lastEventID string
}

func (p *streamParser) feed(b []byte, at time.Duration, events []Event) []Event {
	for _, c := range b {
		if p.skipLF {
			p.skipLF = false
			if c == '\n' {
				continue
			}
		}
		switch c {
		case '\r':
			p.skipLF = true
			events = p.line(at, events)
		case '\n':
			events = p.line(at, events)
		default:
			p.partial = append(p.partial, c)
		}
	}
	return events
}

// flush handles a final line without a newline when the body ends.
func (p *streamParser) flush(at time.Duration, events []Event) []Event {
	if len(p.partial) > 0 {
		events = p.line(at, events)
	}
	if p.kind == "sse" && p.hasData {
		events = p.line(at, events) // a stream that ends mid-event still shows it
	}
	return events
}

func (p *streamParser) line(at time.Duration, events []Event) []Event {
	line := string(p.partial)
	p.partial = p.partial[:0]
	if p.kind == "ndjson" {
		if line != "" {
			events = append(events, Event{At: at, Data: line})
		}
		return events
	}

	if line == "" { // dispatch
		if p.hasData {
			events = append(events, Event{At: at, ID: p.lastEventID, Type: p.eventType, Data: string(p.data)})
		}
		p.data, p.hasData, p.eventType = p.data[:0], false, ""
		return events
	}
	if line[0] == ':' {
		return events // comment, often a keep-alive
	}
	field, value := line, ""
	for i := 0; i < len(line); i++ {
		if line[i] == ':' {
			field, value = line[:i], line[i+1:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
			break
		}
	}
	switch field {
	case "data":
		if p.hasData {
			p.data = append(p.data, '\n')
		}
		p.data = append(p.data, value...)
		p.hasData = true
	case "event":
		p.eventType = value
	case "id":
		p.lastEventID = value
	}
	return events
}

// ParseStream splits a complete streamed body into events, for bodies
// that were saved rather than received live (their times are zero).
func ParseStream(kind string, body []byte) []Event {
	if kind == "" {
		return nil
	}
	p := &streamParser{kind: kind}
	return p.flush(0, p.feed(body, 0, nil))
}
