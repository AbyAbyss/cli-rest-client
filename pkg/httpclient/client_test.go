package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Method", r.Method)
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("PATCH", srv.URL, nil)
	resp, err := NewClient(Options{}).Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 418 || resp.StatusText() != "I'm a teapot" || string(resp.Body) != "short and stout" {
		t.Fatalf("unexpected response %+v", resp)
	}
	if resp.Headers.Get("X-Method") != "PATCH" {
		t.Fatalf("method not sent")
	}
}

func TestRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("end"))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/start", nil)
	resp, err := NewClient(Options{}).Do(context.Background(), req)
	if err != nil || resp.StatusCode != 200 || resp.URL != srv.URL+"/end" {
		t.Fatalf("follow: %v %+v", err, resp)
	}

	req, _ = http.NewRequest("GET", srv.URL+"/start", nil)
	resp, err = NewClient(Options{DisableRedirects: true}).Do(context.Background(), req)
	if err != nil || resp.StatusCode != 302 {
		t.Fatalf("no-follow: %v %+v", err, resp)
	}
}

func TestCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := NewClient(Options{}).Do(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
