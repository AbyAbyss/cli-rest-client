package webui

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/load"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// loadRequest is what the page posts to /api/load.
type loadRequest struct {
	ID    string    `json:"id"` // POST /api/stop with it ends the test early
	Form  load.Form `json:"form"`
	Steps []struct {
		Name    string         `json:"name"`
		Request models.Request `json:"request"`
	} `json:"steps"`
	// Confirmed is set when the page has asked the user about hosts that
	// aren't on this machine or a private network.
	Confirmed bool `json:"confirmed"`
}

// loadTest runs a load test and streams NDJSON:
//
//	{"kind":"confirm","hosts":[...],"plan":"..."}  ask, then post again with confirmed
//	{"kind":"start","plan":"...","steps":[...],"env":"..."}
//	{"kind":"tick","progress":{...}}            about once a second
//	{"kind":"done","summary":{...},"text":"..."}
//	{"kind":"error","error":"..."}
func (s *Server) loadTest(w http.ResponseWriter, r *http.Request) {
	var in loadRequest
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	f, _ := w.(http.Flusher)
	out := &lineWriter{w, f}
	var mu sync.Mutex
	write := func(v any) {
		mu.Lock()
		defer mu.Unlock()
		out.write(v)
	}

	plan, think, ths, err := in.Form.Parse()
	if err != nil {
		write(map[string]any{"kind": "error", "error": err.Error()})
		return
	}
	var steps []load.Step
	for _, st := range in.Steps {
		if st.Request.Type == models.TypeWebSocket {
			continue
		}
		steps = append(steps, load.Step{Name: st.Name, Request: st.Request})
	}
	if len(steps) == 0 {
		write(map[string]any{"kind": "error", "error": "no HTTP requests to send; load tests don't support WebSocket yet"})
		return
	}
	s.mu.Lock()
	vars := s.ws.VariableMap()
	settings := s.ws.Settings
	env := s.ws.ActiveEnvironment
	s.mu.Unlock()

	if remote := load.Remote(load.Hosts(steps, vars)); len(remote) > 0 && !in.Confirmed {
		write(map[string]any{"kind": "confirm", "hosts": remote, "plan": plan.Describe()})
		return
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
	names := make([]string, len(steps))
	for i, st := range steps {
		names[i] = st.Name
	}
	write(map[string]any{"kind": "start", "plan": plan.Describe(), "totalMs": float64(plan.Total()) / float64(time.Millisecond),
		"rate": plan.Rate, "steps": names, "env": env})

	sum, err := load.Run(ctx, load.Config{
		Steps: steps, Plan: plan, Variables: vars, Think: think, Thresholds: ths,
		Timeout:     time.Duration(settings.TimeoutSeconds) * time.Second,
		InsecureTLS: settings.InsecureSkipVerify, DisableRedirects: settings.DisableRedirects,
		Tick: func(p load.Progress) { write(map[string]any{"kind": "tick", "progress": p}) },
	})
	if err != nil {
		write(map[string]any{"kind": "error", "error": err.Error()})
		return
	}
	if r.Context().Err() != nil {
		return // the page went away
	}
	var text bytes.Buffer
	load.WriteSummary(&text, sum)
	write(map[string]any{"kind": "done", "summary": sum, "text": text.String()})
}
