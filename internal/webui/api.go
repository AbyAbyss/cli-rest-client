package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AbyAbyss/cli-rest-client/internal/codegen"
	"github.com/AbyAbyss/cli-rest-client/internal/curl"
	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/graphql"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/postman"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// ---------- JSON shapes ----------

type eventJSON struct {
	AtMs float64 `json:"atMs"`
	ID   string  `json:"id,omitempty"`
	Type string  `json:"type,omitempty"`
	Data string  `json:"data"`
}

func eventsJSON(evs []httpclient.Event) []eventJSON {
	out := make([]eventJSON, len(evs))
	for i, e := range evs {
		out[i] = eventJSON{float64(e.At.Microseconds()) / 1000, e.ID, e.Type, e.Data}
	}
	return out
}

type responseJSON struct {
	Status     int         `json:"status"`
	StatusText string      `json:"statusText"`
	Proto      string      `json:"proto"`
	Headers    http.Header `json:"headers"`
	Body       string      `json:"body"`
	Base64     bool        `json:"base64,omitempty"` // Body is base64: the bytes aren't UTF-8 text
	Size       int         `json:"size"`
	Truncated  bool        `json:"truncated,omitempty"`
	DurationMs float64     `json:"durationMs"`
	URL        string      `json:"url"`
	Stream     string      `json:"stream,omitempty"`
	Events     []eventJSON `json:"events,omitempty"`
	Stopped    bool        `json:"stopped,omitempty"`
}

func bodyJSON(b []byte) (string, bool) {
	if utf8.Valid(b) {
		return string(b), false
	}
	return base64.StdEncoding.EncodeToString(b), true
}

func toResponseJSON(r *httpclient.Response) *responseJSON {
	body, b64 := bodyJSON(r.Body)
	return &responseJSON{
		Status: r.StatusCode, StatusText: r.StatusText(), Proto: r.Proto, Headers: r.Headers,
		Body: body, Base64: b64, Size: len(r.Body), Truncated: r.Truncated,
		DurationMs: float64(r.Duration.Microseconds()) / 1000, URL: r.URL,
		Stream: r.Stream, Events: eventsJSON(r.Events), Stopped: r.Stopped,
	}
}

type testJSON struct {
	Source  string `json:"source"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
	Capture bool   `json:"capture,omitempty"`
}

func testsJSON(rs []script.Result) []testJSON {
	out := make([]testJSON, 0, len(rs))
	for _, r := range rs {
		out = append(out, testJSON{r.Source, r.Passed, r.Message, r.Capture != nil})
	}
	return out
}

// historyItem is a history entry with its body as text.
type historyItem struct {
	*models.HistoryEntry
	Body       *struct{} `json:"body,omitempty"` // hides the base64 []byte; see BodyText
	BodyText   string    `json:"bodyText,omitempty"`
	BodyBase64 bool      `json:"bodyBase64,omitempty"`
}

func historyItems(h *models.History) []historyItem {
	out := make([]historyItem, len(h.Entries))
	for i, e := range h.Entries {
		item := historyItem{HistoryEntry: e}
		item.BodyText, item.BodyBase64 = bodyJSON(e.Body)
		out[i] = item
	}
	return out
}

type langJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func languages() []langJSON {
	out := make([]langJSON, len(codegen.Languages))
	for i, l := range codegen.Languages {
		out[i] = langJSON{l.ID, l.Name}
	}
	return out
}

// ---------- send ----------

// sendRequest is what the page posts to /api/send.
type sendRequest struct {
	Request models.Request `json:"request"`
	Source  string         `json:"source"` // "Collection / Folder / Request", for history
	// ID names the send so POST /api/stop can end it while the page keeps
	// reading, and still gets the response and the tests.
	ID string `json:"id"`
}

// line writes one NDJSON event and flushes it to the page.
type lineWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (l *lineWriter) write(v any) {
	b, _ := json.Marshal(v)
	l.w.Write(append(b, '\n'))
	if l.f != nil {
		l.f.Flush()
	}
}

// send runs a request and answers with NDJSON events as things happen:
//
//	{"kind":"start", ...}   what is being sent, with variables resolved
//	{"kind":"head", ...}    a streaming response's status and headers
//	{"kind":"events", ...}  stream events as they arrive
//	{"kind":"done", ...}    the response, the tests, and the workspace if variables changed
//	{"kind":"error", ...}   nothing came back
//
// Closing the connection (the page's Stop button) ends a stream, keeping
// what arrived.
func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	var in sendRequest
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req := in.Request
	req.Normalize()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	f, _ := w.(http.Flusher)
	out := &lineWriter{w, f}

	s.mu.Lock()
	vars := s.ws.VariableMap()
	settings := s.ws.Settings
	env := s.ws.ActiveEnvironment
	s.mu.Unlock()

	changed := false
	if strings.TrimSpace(req.PreRequest) != "" {
		assignments, errs := script.RunPre(req.PreRequest, vars)
		changed = s.apply(assignments) || changed
		if len(errs) > 0 {
			msgs := make([]string, len(errs))
			for i, e := range errs {
				msgs[i] = e.Error()
			}
			out.write(map[string]any{"kind": "error", "error": "pre-request script: " + strings.Join(msgs, "; "), "env": env})
			return
		}
	}
	p, err := engine.Prepare(req, vars)
	if err != nil {
		out.write(map[string]any{"kind": "error", "error": err.Error(), "env": env})
		return
	}
	started := time.Now()
	sent := p.SentHeaders()
	out.write(map[string]any{
		"kind": "start", "method": p.Request.Method, "url": p.Request.URL.String(), "env": env,
		"missing": p.Missing, "warnings": p.Warnings, "requestHeaders": sent,
	})

	client := httpclient.NewClient(httpclient.Options{
		Timeout:            time.Duration(settings.TimeoutSeconds) * time.Second,
		DisableRedirects:   settings.DisableRedirects,
		InsecureSkipVerify: settings.InsecureSkipVerify,
	})
	headSent, eventsSent := false, 0
	update := func(snap *httpclient.Response) {
		if snap.Stream == "" {
			return
		}
		if !headSent {
			headSent = true
			out.write(map[string]any{"kind": "head", "status": snap.StatusCode, "statusText": snap.StatusText(),
				"proto": snap.Proto, "headers": snap.Headers, "stream": snap.Stream})
		}
		if len(snap.Events) > eventsSent {
			out.write(map[string]any{"kind": "events", "events": eventsJSON(snap.Events[eventsSent:]),
				"size": len(snap.Body), "elapsedMs": float64(snap.Duration.Microseconds()) / 1000})
			eventsSent = len(snap.Events)
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if in.ID != "" {
		s.mu.Lock()
		s.running[in.ID] = cancel
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			delete(s.running, in.ID)
			s.mu.Unlock()
		}()
	}
	resp, err := client.DoStream(ctx, p.Request, update)

	entry := &models.HistoryEntry{
		Time: started, Request: in.Request, Source: in.Source, Environment: env,
		Method: p.Request.Method, URL: p.Request.URL.String(), RequestHeaders: sent,
	}
	if err != nil {
		if r.Context().Err() != nil {
			return // the page went away before anything came back
		}
		if errors.Is(err, context.Canceled) {
			err = errors.New("stopped before a response arrived")
		}
		entry.Error = err.Error()
		entry.DurationMs = time.Since(started).Milliseconds()
		s.record(entry, settings.DisableHistory)
		out.write(map[string]any{"kind": "error", "error": err.Error(), "env": env, "history": s.historySnapshot()})
		return
	}

	var tests []script.Result
	if strings.TrimSpace(req.Tests) != "" {
		events := make([]string, len(resp.Events))
		for i, e := range resp.Events {
			events[i] = e.Data
		}
		tests = script.RunTests(req.Tests, script.Response{
			Status: resp.StatusCode, Headers: resp.Headers, Body: resp.Body, Duration: resp.Duration, Events: events,
		}, vars)
		var captures []script.Assignment
		for _, t := range tests {
			if t.Capture != nil {
				captures = append(captures, *t.Capture)
			}
		}
		changed = s.apply(captures) || changed
	}

	entry.Status, entry.StatusText, entry.Proto = resp.StatusCode, resp.StatusText(), resp.Proto
	entry.DurationMs = resp.Duration.Milliseconds()
	entry.ResponseHeaders, entry.Body, entry.BodySize, entry.BodyTruncated = resp.Headers, resp.Body, len(resp.Body), resp.Truncated
	if resp.URL != entry.URL {
		entry.FinalURL = resp.URL
	}
	for _, t := range tests {
		entry.Tests = append(entry.Tests, models.TestOutcome{Source: t.Source, Passed: t.Passed, Message: t.Message, Capture: t.Capture != nil})
	}
	s.record(entry, settings.DisableHistory)

	done := map[string]any{"kind": "done", "response": toResponseJSON(resp), "tests": testsJSON(tests), "history": s.historySnapshot()}
	if changed {
		s.mu.Lock()
		done["workspace"] = s.ws
		s.mu.Unlock()
	}
	out.write(done)
}

// stop ends a send started with an ID (the page's Stop button).
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	cancel, ok := s.running[in.ID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
	writeJSON(w, map[string]bool{"stopped": ok})
}

// apply saves script assignments to the workspace and reports whether any
// were made.
func (s *Server) apply(as []script.Assignment) bool {
	if len(as) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range as {
		if a.Unset {
			s.ws.UnsetVariable(a.Name)
		} else {
			s.ws.SetVariable(a.Name, a.Value)
		}
	}
	_ = s.save()
	return true
}

func (s *Server) record(e *models.HistoryEntry, disabled bool) {
	if disabled {
		return
	}
	s.mu.Lock()
	s.hist.Add(e)
	s.saveHistory()
	s.mu.Unlock()
}

func (s *Server) historySnapshot() []historyItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return historyItems(s.hist)
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.hist.Entries = nil
	s.saveHistory()
	s.mu.Unlock()
	writeJSON(w, map[string]any{"history": []historyItem{}})
}

func (s *Server) deleteHistory(w http.ResponseWriter, r *http.Request) {
	i, err := strconv.Atoi(r.PathValue("i"))
	s.mu.Lock()
	if err == nil && i >= 0 && i < len(s.hist.Entries) {
		s.hist.Entries = append(s.hist.Entries[:i], s.hist.Entries[i+1:]...)
		s.saveHistory()
	}
	s.mu.Unlock()
	writeJSON(w, map[string]any{"history": s.historySnapshot()})
}

// ---------- code, schema, import, export ----------

func (s *Server) code(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Request  models.Request `json:"request"`
		Lang     string         `json:"lang"`
		Template bool           `json:"template"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	vars := s.ws.VariableMap()
	s.mu.Unlock()
	snip, err := codegen.Generate(in.Lang, in.Request, vars, in.Template)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"text": snip.Text, "missing": snip.Missing})
}

type schemaField struct {
	graphql.Field
	TypeString string            `json:"typeString"`
	ArgTypes   map[string]string `json:"argTypes,omitempty"`
	Named      string            `json:"named"`
	Query      string            `json:"query,omitempty"`
	Variables  string            `json:"variables,omitempty"`
}

type schemaType struct {
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	Description string        `json:"description,omitempty"`
	Fields      []schemaField `json:"fields"`
}

func (s *Server) schema(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Request models.Request `json:"request"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req := in.Request
	req.Type, req.Method, req.BodyType = models.TypeHTTP, "POST", models.BodyGraphQL
	req.Body, req.GraphQLVariables = graphql.IntrospectionQuery, ""
	s.mu.Lock()
	vars, settings := s.ws.VariableMap(), s.ws.Settings
	s.mu.Unlock()
	p, err := engine.Prepare(req, vars)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client := httpclient.NewClient(httpclient.Options{
		Timeout:            time.Duration(settings.TimeoutSeconds) * time.Second,
		InsecureSkipVerify: settings.InsecureSkipVerify,
	})
	resp, err := client.Do(r.Context(), p.Request)
	if err == nil && resp.StatusCode >= 400 {
		err = fmt.Errorf("the server answered %s", resp.Status)
	}
	var sc *graphql.Schema
	if err == nil {
		sc, err = graphql.Parse(resp.Body)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	types := map[string]schemaType{}
	roots := map[string]string{}
	for _, rt := range sc.Roots() {
		roots[rt[0]] = rt[1]
	}
	for name, t := range sc.Types {
		st := schemaType{Name: name, Kind: t.Kind, Description: t.Description}
		op := ""
		for o, root := range roots {
			if root == name {
				op = o
			}
		}
		for _, f := range t.Fields {
			sf := schemaField{Field: f, TypeString: f.Type.String()}
			if n := f.Type.Named(); n != nil {
				sf.Named = n.Name
			}
			if len(f.Args) > 0 {
				sf.ArgTypes = map[string]string{}
				for _, a := range f.Args {
					sf.ArgTypes[a.Name] = a.Type.String()
				}
			}
			if op != "" {
				sf.Query, sf.Variables = sc.Skeleton(op, f)
			}
			st.Fields = append(st.Fields, sf)
		}
		types[name] = st
	}
	writeJSON(w, map[string]any{"url": p.Request.URL.String(), "roots": roots, "types": types})
}

// importData takes curl commands or a Postman export. A single curl
// command comes back as a request to open; anything else is added to the
// workspace.
func (s *Server) importData(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	text := strings.TrimSpace(in.Text)
	if curl.LooksLikeCurl(text) {
		results, err := curl.ParseAll(text)
		if err != nil {
			writeError(w, http.StatusBadRequest, "curl: "+err.Error())
			return
		}
		var notes []string
		for _, res := range results {
			notes = append(notes, res.Notes...)
		}
		if len(results) == 1 {
			writeJSON(w, map[string]any{"request": results[0].Request, "notes": notes,
				"summary": "Imported from cURL: " + results[0].Request.Method + " " + results[0].Request.URL})
			return
		}
		c := &models.Collection{Name: "cURL import"}
		for i, res := range results {
			req := res.Request
			if req.Name == "" {
				req.Name = fmt.Sprintf("Request %d", i+1)
			}
			c.Requests = append(c.Requests, &req)
		}
		s.mu.Lock()
		s.ws.Collections = append(s.ws.Collections, c)
		err = s.save()
		ws := s.ws
		s.mu.Unlock()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"workspace": ws, "notes": notes,
			"summary": fmt.Sprintf("Imported %d curl commands into the collection \"cURL import\"", len(results))})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := postman.Import([]byte(text), s.ws)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"workspace": s.ws, "notes": res.Warnings, "summary": res.Summary()})
}

// export writes a collection or folder (by its index path) as a Postman
// collection.
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Folders []int `json:"folders"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var c *models.Collection
	list := s.ws.Collections
	for _, i := range in.Folders {
		if i < 0 || i >= len(list) {
			writeError(w, http.StatusBadRequest, "no such collection or folder")
			return
		}
		c = list[i]
		list = c.Folders
	}
	if c == nil {
		writeError(w, http.StatusBadRequest, "pick a collection or folder")
		return
	}
	res, err := postman.ExportCollection(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"filename": c.Name + ".postman_collection.json", "data": string(res.Data), "notes": res.Notes})
}
