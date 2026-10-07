// Package httpclient sends HTTP requests and captures timing and body data.
package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxBodySize caps how much of a response body is read into memory.
const MaxBodySize = 50 << 20 // 50 MiB

// Options configures a Client.
type Options struct {
	// Timeout for the whole exchange. Zero means 30 seconds.
	Timeout time.Duration
	// DisableRedirects returns 3xx responses instead of following them.
	DisableRedirects bool
	// InsecureSkipVerify disables TLS certificate verification.
	InsecureSkipVerify bool
}

// Client wraps an http.Client.
type Client struct {
	httpClient *http.Client
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
	c := &http.Client{Timeout: opts.Timeout, Transport: transport}
	if opts.DisableRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &Client{httpClient: c}
}

// Response is a fully read HTTP response.
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

// Do sends req and reads the whole response body.
func (c *Client) Do(ctx context.Context, req *http.Request) (*Response, error) {
	start := time.Now()
	resp, err := c.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	truncated := false
	if len(data) > MaxBodySize {
		data = data[:MaxBodySize]
		truncated = true
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Proto:      resp.Proto,
		Headers:    resp.Header,
		Body:       data,
		Truncated:  truncated,
		Duration:   time.Since(start),
		URL:        resp.Request.URL.String(),
	}, nil
}
