// Package testutil provides a small httpbin.org look-alike for tests.
package testutil

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// NewHTTPBin starts a server implementing the httpbin endpoints used by the
// sample workspace: /get /post /put /patch /delete /json /bearer
// /basic-auth/{user}/{pass}, plus the streaming endpoints in streams.go.
func NewHTTPBin() *httptest.Server {
	mux := http.NewServeMux()
	echo := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		args := map[string]string{}
		for k, v := range r.URL.Query() {
			args[k] = v[0]
		}
		headers := map[string]string{}
		for k, v := range r.Header {
			headers[k] = strings.Join(v, ", ")
		}
		out := map[string]any{"args": args, "headers": headers, "url": r.URL.String(), "method": r.Method, "data": string(body)}
		form := map[string]string{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			_ = r.ParseForm()
			for k, v := range r.PostForm {
				form[k] = v[0]
			}
		}
		out["form"] = form
		var js any
		if json.Unmarshal(body, &js) == nil {
			out["json"] = js
		} else {
			out["json"] = nil
		}
		writeJSON(w, http.StatusOK, out)
	}
	for _, p := range []string{"/get", "/post", "/put", "/patch", "/delete", "/anything"} {
		mux.HandleFunc(p, echo)
	}
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"slideshow": map[string]any{"title": "Sample Slide Show", "slides": []any{map[string]any{"title": "Wake up"}}}})
	})
	mux.HandleFunc("/bearer", func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "token": tok})
	})
	mux.HandleFunc("/basic-auth/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/basic-auth/"), "/")
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.Join(parts, ":")))
		if len(parts) != 2 || r.Header.Get("Authorization") != want {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user": parts[0]})
	})
	addStreams(mux)
	addGraphQL(mux)
	addWebSocket(mux)
	return httptest.NewServer(mux)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// UseLocal points the sample workspace's URL variables at a server from
// NewHTTPBin, so its requests (including the GraphQL and SSE samples) run
// without the internet.
func UseLocal(ws *models.Workspace, base string) {
	ws.SetVariable("baseUrl", base)
	ws.SetVariable("graphqlUrl", base+"/graphql")
	ws.SetVariable("sseUrl", base+"/sse?count=3&interval=10")
	ws.SetVariable("wsUrl", "ws"+strings.TrimPrefix(base, "http")+"/ws")
}
