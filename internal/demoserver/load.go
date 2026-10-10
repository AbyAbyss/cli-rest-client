package demoserver

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// addLoad adds endpoints for trying load tests: a slow one, a flaky one
// and one that returns any status.
//
//	/slow?ms=80&jitter=40   answers after 80ms ± 40ms
//	/flaky?fail=5&ms=20     5% of answers are 500s (after ~20ms)
//	/status/503             answers with that status
func addLoad(mux *http.ServeMux) {
	wait := func(r *http.Request, def int) time.Duration {
		ms := intParam(r, "ms", def)
		if j := intParam(r, "jitter", 0); j > 0 {
			ms += rand.IntN(2*j+1) - j
		}
		d := time.Duration(max(ms, 0)) * time.Millisecond
		select {
		case <-time.After(d):
		case <-r.Context().Done():
		}
		return d
	}
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		d := wait(r, 100)
		writeJSON(w, http.StatusOK, map[string]any{"slept_ms": d.Milliseconds()})
	})
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		d := wait(r, 10)
		if rand.Float64()*100 < float64(intParam(r, "fail", 10)) {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "flaky failure"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slept_ms": d.Milliseconds()})
	})
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/status/"))
		if err != nil || code < 100 || code > 599 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "use /status/<100-599>"})
			return
		}
		w.WriteHeader(code)
	})
}

func intParam(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return def
}
