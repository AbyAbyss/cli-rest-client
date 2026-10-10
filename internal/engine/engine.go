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
	// Message is the resolved message of a WebSocket request. Request is
	// then the handshake: a GET to the ws:// or wss:// URL.
	Message string
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
	ws := req.Type == models.TypeWebSocket
	if !strings.Contains(rawURL, "://") {
		if ws {
			rawURL = "ws://" + rawURL
		} else {
			rawURL = "http://" + rawURL
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if ws {
		switch u.Scheme { // http(s) URLs are accepted, as browsers do
		case "http":
			u.Scheme = "ws"
		case "https":
			u.Scheme = "wss"
		case "ws", "wss":
		default:
			return nil, fmt.Errorf("unsupported URL scheme %q for a WebSocket (use ws or wss)", u.Scheme)
		}
	} else if u.Scheme != "http" && u.Scheme != "https" {
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

	if ws {
		// The handshake carries no body; the body is the first message.
		p.Message = r.sub(req.Body)
		req.Method, req.BodyType = http.MethodGet, models.BodyNone
	}

	var graphQL []byte // the POST body of a GraphQL request
	if req.BodyType == models.BodyGraphQL {
		q, variables, err := graphQLParts(r.sub(req.Body), r.sub(req.GraphQLVariables))
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(strings.TrimSpace(req.Method), http.MethodGet) {
			// GraphQL over HTTP: a GET carries the query in the URL.
			addQuery("query", q)
			if variables != nil {
				addQuery("variables", string(variables))
			}
		} else {
			graphQL = graphQLBody(q, variables)
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
	if graphQL != nil {
		p.Body = graphQL
		contentType = "application/json"
	}
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

// graphQLParts checks a GraphQL query and its variables. Variables must be
// empty or a JSON object; they are returned compacted.
func graphQLParts(query, variables string) (string, json.RawMessage, error) {
	if strings.TrimSpace(query) == "" {
		return "", nil, errors.New("the GraphQL query is empty")
	}
	variables = strings.TrimSpace(variables)
	if variables == "" {
		return query, nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(variables), &obj); err != nil {
		return "", nil, fmt.Errorf("GraphQL variables must be a JSON object: %v", err)
	}
	var buf bytes.Buffer
	_ = json.Compact(&buf, []byte(variables))
	return query, buf.Bytes(), nil
}

// graphQLBody is the JSON body of a GraphQL POST.
func graphQLBody(query string, variables json.RawMessage) []byte {
	body := struct {
		Query     string          `json:"query"`
		Variables json.RawMessage `json:"variables,omitempty"`
	}{query, variables}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
