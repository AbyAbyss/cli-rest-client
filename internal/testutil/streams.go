package testutil

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// addStreams registers streaming endpoints:
//
//	/sse?count=N&interval=MS     Server-Sent Events (count 0 streams until the client leaves)
//	/ndjson?count=N&interval=MS  newline-delimited JSON
func addStreams(mux *http.ServeMux) {
	params := func(r *http.Request) (int, time.Duration) {
		count, err := strconv.Atoi(r.URL.Query().Get("count"))
		if err != nil || count < 0 {
			count = 5
		}
		ms, err := strconv.Atoi(r.URL.Query().Get("interval"))
		if err != nil || ms < 0 {
			ms = 200
		}
		return count, time.Duration(ms) * time.Millisecond
	}
	stream := func(w http.ResponseWriter, r *http.Request, contentType string, write func(i int)) {
		count, interval := params(r)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		f, _ := w.(http.Flusher)
		for i := 1; count == 0 || i <= count; i++ {
			if i > 1 {
				select {
				case <-time.After(interval):
				case <-r.Context().Done():
					return
				}
			}
			write(i)
			if f != nil {
				f.Flush()
			}
		}
	}
	steps := []string{"queued", "building", "testing", "deploying", "live"}
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		stream(w, r, "text/event-stream", func(i int) {
			if i == 1 {
				fmt.Fprint(w, ": connected\n\n")
			}
			fmt.Fprintf(w, "event: progress\nid: %d\ndata: {\"step\":%d,\"status\":%q}\n\n", i, i, steps[(i-1)%len(steps)])
		})
	})
	mux.HandleFunc("/ndjson", func(w http.ResponseWriter, r *http.Request) {
		stream(w, r, "application/x-ndjson", func(i int) {
			fmt.Fprintf(w, "{\"n\":%d,\"status\":%q}\n", i, steps[(i-1)%len(steps)])
		})
	})
}
