package ui

import (
	"net/http"
	"net/http/httptest"
)

func newSlowServer(block chan struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
}
