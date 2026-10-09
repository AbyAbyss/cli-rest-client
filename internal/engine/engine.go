// Package engine turns a saved request plus variables into a ready-to-send
// *http.Request, and renders it as a cURL command.
package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/vars"
)

// Prepared is a request with variables resolved.
type Prepared struct {
	Request *http.Request
	Body    []byte
	// Missing lists {{variables}} that had no value.
	Missing []string
	// Warnings are non-fatal problems, such as an invalid JSON body.
	Warnings []string
}

type resolver struct {
	vars    map[string]string
	missing map[string]bool
}

func (r *resolver) sub(s string) string {
	out, missing := vars.Substitute(s, r.vars)
	for _, m := range missing {
		r.missing[m] = true
	}
	return out
}

// Prepare resolves variables and builds the HTTP request.
func Prepare(req models.Request, variables map[string]string) (*Prepared, error) {
	req.Normalize()
	r := &resolver{vars: variables, missing: map[string]bool{}}
	p := &Prepared{}

	rawURL := strings.TrimSpace(r.sub(req.URL))
	if rawURL == "" {
		return nil, errors.New("URL is empty")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q (use http or https)", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL %q has no host", rawURL)
	}

	var query []string
	addQuery := func(k, v string) {
		query = append(query, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	for _, kv := range req.Params {
		if kv.Disabled || kv.Key == "" {
			continue
		}
		addQuery(r.sub(kv.Key), r.sub(kv.Value))
	}

	headers := http.Header{}
	for _, kv := range req.Headers {
		if kv.Disabled || kv.Key == "" {
			continue
		}
		name := r.sub(kv.Key)
		if !validHeaderName(name) {
			return nil, fmt.Errorf("invalid header name %q", name)
		}
		headers.Add(name, r.sub(kv.Value))
	}

	switch req.Auth.Type {
	case models.AuthBasic:
		creds := r.sub(req.Auth.Username) + ":" + r.sub(req.Auth.Password)
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	case models.AuthBearer:
		headers.Set("Authorization", "Bearer "+r.sub(req.Auth.Token))
	case models.AuthAPIKey:
		key := r.sub(req.Auth.Key)
		if key == "" {
			return nil, errors.New("API key auth needs a key name")
		}
		if req.Auth.In == "query" {
			addQuery(key, r.sub(req.Auth.Value))
		} else {
			if !validHeaderName(key) {
				return nil, fmt.Errorf("invalid API key header name %q", key)
			}
			headers.Set(key, r.sub(req.Auth.Value))
		}
	}

	if len(query) > 0 {
		extra := strings.Join(query, "&")
		if u.RawQuery == "" {
			u.RawQuery = extra
		} else {
			u.RawQuery += "&" + extra
		}
	}

	var contentType string
	switch req.BodyType {
	case models.BodyJSON:
		p.Body = []byte(r.sub(req.Body))
		contentType = "application/json"
		if len(bytes.TrimSpace(p.Body)) > 0 && !json.Valid(p.Body) {
			p.Warnings = append(p.Warnings, "body is not valid JSON")
		}
	case models.BodyText:
		p.Body = []byte(r.sub(req.Body))
		contentType = "text/plain; charset=utf-8"
	case models.BodyXML:
		p.Body = []byte(r.sub(req.Body))
		contentType = "application/xml"
	case models.BodyForm:
		form := url.Values{}
		var keys []string
		for _, kv := range models.ParseKV(req.Body, "=") {
			if kv.Disabled || kv.Key == "" {
				continue
			}
			k := r.sub(kv.Key)
			if _, ok := form[k]; !ok {
				keys = append(keys, k)
			}
			form.Add(k, r.sub(kv.Value))
		}
		// Keep the user's field order instead of url.Values.Encode's sorted order.
		var parts []string
		for _, k := range keys {
			for _, v := range form[k] {
				parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		p.Body = []byte(strings.Join(parts, "&"))
		contentType = "application/x-www-form-urlencoded"
	}
	if contentType != "" && headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", contentType)
	}
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", "term-rest-client")
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	httpReq, err := http.NewRequest(method, u.String(), bytes.NewReader(p.Body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if len(p.Body) == 0 {
		httpReq.Body = http.NoBody
		httpReq.ContentLength = 0
	}
	httpReq.Header = headers
	if host := headers.Get("Host"); host != "" {
		httpReq.Host = host
	}
	p.Request = httpReq

	for m := range r.missing {
		p.Missing = append(p.Missing, m)
	}
	sort.Strings(p.Missing)
	return p, nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c <= ' ' || c >= 0x7f || strings.ContainsRune("\"(),/:;<=>?@[\\]{}", c) {
			return false
		}
	}
	return true
}

// SentHeaders returns the headers that go on the wire: the request's own
// headers plus the ones Go's HTTP transport adds by itself (Host,
// Content-Length, and Accept-Encoding when compression is negotiated).
func (p *Prepared) SentHeaders() http.Header {
	h := p.Request.Header.Clone()
	if h.Get("Host") == "" {
		h.Set("Host", p.Request.URL.Host)
	}
	switch {
	case len(p.Body) > 0:
		h.Set("Content-Length", strconv.Itoa(len(p.Body)))
	case p.Request.Method == http.MethodPost || p.Request.Method == http.MethodPut || p.Request.Method == http.MethodPatch:
		h.Set("Content-Length", "0")
	}
	if h.Get("Accept-Encoding") == "" && h.Get("Range") == "" && p.Request.Method != http.MethodHead {
		h.Set("Accept-Encoding", "gzip")
	}
	return h
}

// Curl renders the prepared request as a cURL command line.
func (p *Prepared) Curl() string {
	var sb strings.Builder
	sb.WriteString("curl")
	switch p.Request.Method {
	case http.MethodGet:
	case http.MethodHead:
		sb.WriteString(" -I") // -X HEAD would make curl wait for a body
	default:
		sb.WriteString(" -X " + p.Request.Method)
	}
	sb.WriteString(" " + shellQuote(p.Request.URL.String()))
	names := make([]string, 0, len(p.Request.Header))
	for k := range p.Request.Header {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		for _, v := range p.Request.Header[k] {
			sb.WriteString(" \\\n  -H " + shellQuote(k+": "+v))
		}
	}
	if len(p.Body) > 0 {
		sb.WriteString(" \\\n  --data-raw " + shellQuote(string(p.Body)))
	}
	return sb.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
