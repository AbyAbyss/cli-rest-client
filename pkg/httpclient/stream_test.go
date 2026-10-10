package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSSEParser(t *testing.T) {
	stream := ": keep-alive\r\n" +
		"data: first\r\n\r\n" +
		"event: update\nid: 7\ndata: line one\ndata: line two\n\n" +
		"data:no space\rdata:  two spaces\r\r" +
		"retry: 1000\n\n" + // no data: nothing dispatched
		"data: {\"n\":1}\n" +
		"id: 8\n\n" +
		"data: unterminated"
	want := []Event{
		{Data: "first"},
		{ID: "7", Type: "update", Data: "line one\nline two"},
		{ID: "7", Data: "no space\n two spaces"},
		{ID: "8", Data: `{"n":1}`},
		{ID: "8", Data: "unterminated"},
	}
	// Feed it whole and one byte at a time: chunk boundaries (even inside
	// \r\n) must not change the result.
	for _, size := range []int{len(stream), 1, 3} {
		p := &streamParser{kind: "sse"}
		var got []Event
		for i := 0; i < len(stream); i += size {
			got = p.feed([]byte(stream[i:min(i+size, len(stream))]), 0, got)
		}
		got = p.flush(0, got)
		if len(got) != len(want) {
			t.Fatalf("chunk %d: got %d events %+v", size, len(got), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("chunk %d, event %d: got %+v, want %+v", size, i, got[i], want[i])
			}
		}
	}
}

func TestNDJSONParser(t *testing.T) {
	p := &streamParser{kind: "ndjson"}
	got := p.feed([]byte("{\"a\":1}\n\n{\"a\":"), 0, nil)
	got = p.feed([]byte("2}\r\n{\"a\":3}"), 0, got)
	got = p.flush(0, got)
	if len(got) != 3 || got[0].Data != `{"a":1}` || got[1].Data != `{"a":2}` || got[2].Data != `{"a":3}` {
		t.Fatalf("got %+v", got)
	}
}

func TestStreamKind(t *testing.T) {
	for ct, want := range map[string]string{
		"text/event-stream":                "sse",
		"text/event-stream; charset=utf-8": "sse",
		"application/x-ndjson":             "ndjson",
		"application/jsonl":                "ndjson",
		"application/json":                 "",
		"":                                 "",
	} {
		if got := StreamKind(ct); got != want {
			t.Errorf("StreamKind(%q) = %q, want %q", ct, got, want)
		}
	}
}

// sseServer sends n events, one every gap.
func sseServer(n int, gap time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for i := 0; i < n || n < 0; i++ {
			if _, err := w.Write([]byte("data: tick\n\n")); err != nil {
				return
			}
			f.Flush()
			select {
			case <-time.After(gap):
			case <-r.Context().Done():
				return
			}
		}
	}))
}

func TestDoStreamOutlivesTimeout(t *testing.T) {
	srv := sseServer(5, 60*time.Millisecond) // 300 ms, timeout is 100 ms
	defer srv.Close()

	var mu sync.Mutex
	var seen []int
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := NewClient(Options{Timeout: 100 * time.Millisecond}).DoStream(context.Background(), req, func(r *Response) {
		mu.Lock()
		seen = append(seen, len(r.Events))
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Stream != "sse" || len(resp.Events) != 5 || !resp.Done || resp.Stopped {
		t.Fatalf("got %+v", resp)
	}
	if resp.Events[4].At <= resp.Events[0].At {
		t.Errorf("event times should increase: %v", resp.Events)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 5 || seen[0] != 0 || seen[len(seen)-1] != 5 {
		t.Errorf("updates should go from 0 to 5 events: %v", seen)
	}

	// Do keeps the timeout for the whole exchange.
	req, _ = http.NewRequest("GET", srv.URL, nil)
	if _, err := NewClient(Options{Timeout: 100 * time.Millisecond}).Do(context.Background(), req); err == nil || !strings.Contains(err.Error(), "timed out after 100ms") {
		t.Fatalf("Do should time out, got %v", err)
	}
}

func TestDoStreamStop(t *testing.T) {
	srv := sseServer(-1, 20*time.Millisecond) // never ends
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := NewClient(Options{}).DoStream(ctx, req, func(r *Response) {
		if len(r.Events) == 3 {
			cancel()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Stopped || len(resp.Events) < 3 || !strings.HasPrefix(string(resp.Body), "data: tick\n\ndata: tick\n\ndata: tick") {
		t.Fatalf("got stopped=%v events=%d body=%q", resp.Stopped, len(resp.Events), resp.Body)
	}
}

func TestOrdinaryResponseTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := NewClient(Options{Timeout: 80 * time.Millisecond}).DoStream(context.Background(), req, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("a non-stream body must stay under the timeout, got %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("a timeout is not a cancel")
	}
}
