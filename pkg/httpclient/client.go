package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client wraps HTTP client functionality
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new HTTP client with default timeout
func NewClient(timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Status     string
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
}

// SendRequest sends an HTTP request and returns the response
func (c *Client) SendRequest(method, url, body string) (*Response, error) {
	start := time.Now()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error building request: %w", err)
	}

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    resp.Header,
		Body:       data,
		Duration:   time.Since(start),
	}, nil
}

// FormatResponse formats a response for display
func FormatResponse(resp *Response) string {
	var sb strings.Builder
	// Use Catppuccin Mocha colors: success green for 2xx/3xx, red for errors
	statusColor := "[#a6e3a1]" // Success green (Catppuccin Mocha)
	if resp.StatusCode >= 400 {
		statusColor = "[#f38ba8]" // Error red (Catppuccin Mocha)
	}
	sb.WriteString(fmt.Sprintf("[#cdd6f4]Status:%s %d %s  [#cdd6f4]Time: [#89b4fa]%.0fms[-]\n",
		statusColor, resp.StatusCode, resp.Status, float64(resp.Duration.Milliseconds())))
	sb.WriteString("[#89b4fa]--- Headers ---[#cdd6f4]\n")
	for k, v := range resp.Headers {
		sb.WriteString(fmt.Sprintf("%s: %s\n", k, strings.Join(v, ", ")))
	}
	sb.WriteString("\n[#89b4fa]--- Body ---[#cdd6f4]\n")
	sb.Write(resp.Body)
	return sb.String()
}
