// Package webui serves the browser interface (term-rest-client --gui).
//
// The page is plain HTML, CSS and JavaScript embedded in the binary. Every
// request it makes is executed here, by the same engine the terminal app
// uses, so there is no CORS and streams, WebSockets, scripts, tests,
// environments and history behave exactly as in the terminal.
//
// By default the server listens on the loopback interface only (-lan opens
// it to the network). A random token, handed
// to the browser in the URL the command prints and kept in a SameSite=Strict
// cookie, guards the API; the Host and Origin headers are checked too, so
// other pages open in the browser can't drive it.
package webui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
)

//go:embed static
var staticFiles embed.FS

const cookieName = "trc_token"

// Info describes the running binary and its files, for the About panel.
type Info struct {
	Version     string `json:"version"`
	BuildTime   string `json:"buildTime"`
	DataPath    string `json:"dataPath"`
	HistoryPath string `json:"historyPath"`
}

// Server is the web UI's HTTP server.
type Server struct {
	mu    sync.Mutex
	ws    *models.Workspace
	store *storage.Store // nil keeps everything in memory (tests)
	hist  *models.History
	info  Info
	token string
	hosts map[string]bool // accepted Host headers
	base  string          // http://127.0.0.1:port

	running map[string]context.CancelFunc // sends in flight, by the page's ID
}

// New creates a server for ws. base is the address the browser uses,
// such as http://127.0.0.1:7300; Host and Origin must match it.
func New(ws *models.Workspace, store *storage.Store, info Info, base string) (*Server, error) {
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	s := &Server{ws: ws, store: store, info: info, token: hex.EncodeToString(tok), hist: &models.History{},
		running: map[string]context.CancelFunc{}}
	if store != nil {
		if h, err := store.LoadHistory(); err == nil {
			s.hist = h
		}
		info.DataPath, info.HistoryPath = store.Path, store.HistoryPath()
		s.info = info
	}
	if err := s.setBase(base); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) setBase(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return fmt.Errorf("bad address %q", base)
	}
	s.base = strings.TrimSuffix(base, "/")
	s.hosts = map[string]bool{u.Host: true}
	if _, port, err := net.SplitHostPort(u.Host); err == nil {
		s.hosts["localhost:"+port] = true
		s.hosts["127.0.0.1:"+port] = true
	}
	return nil
}

// LoginURL is the URL to open: it carries the token once, then the page
// keeps it in a cookie.
func (s *Server) LoginURL() string { return s.base + "/?t=" + s.token }

// Token is the API token, for tests.
func (s *Server) Token() string { return s.token }

// Handler routes requests.
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(staticFiles, "static")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", noCache(files)))
	mux.HandleFunc("GET /api/state", s.api(s.state))
	mux.HandleFunc("PUT /api/workspace", s.api(s.putWorkspace))
	mux.HandleFunc("POST /api/send", s.api(s.send))
	mux.HandleFunc("POST /api/stop", s.api(s.stop))
	mux.HandleFunc("GET /api/socket", s.api(s.socket))
	mux.HandleFunc("POST /api/code", s.api(s.code))
	mux.HandleFunc("POST /api/schema", s.api(s.schema))
	mux.HandleFunc("POST /api/import", s.api(s.importData))
	mux.HandleFunc("POST /api/export", s.api(s.export))
	mux.HandleFunc("DELETE /api/history", s.api(s.clearHistory))
	mux.HandleFunc("DELETE /api/history/{i}", s.api(s.deleteHistory))
	return s.guard(mux)
}

// guard rejects requests for any other host name (DNS rebinding).
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[r.Host] {
			http.Error(w, "unexpected Host header", http.StatusMisdirectedRequest)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// api checks the token (cookie, or X-TRC-Token header) and, for requests
// with a body, that they come from this page.
func (s *Server) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-TRC-Token")
		if c, err := r.Cookie(cookieName); err == nil && tok == "" {
			tok = c.Value
		}
		if tok != s.token {
			writeError(w, http.StatusUnauthorized, "open the link that term-rest-client --gui printed")
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !s.sameOrigin(o) {
			writeError(w, http.StatusForbidden, "cross-origin request refused")
			return
		}
		h(w, r)
	}
}

func (s *Server) sameOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http") && s.hosts[u.Host]
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if t := r.URL.Query().Get("t"); t != "" {
		if t != s.token {
			http.Error(w, "This link is out of date. Use the one term-rest-client --gui printed.", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if c, err := r.Cookie(cookieName); err != nil || c.Value != s.token {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>term-rest-client</title>`+
			`<body style="font:16px system-ui;padding:3rem;background:#09090b;color:#fafafa">`+
			`<h1>term-rest-client</h1><p>Open the link that <code>term-rest-client --gui</code> printed in your terminal. It includes a one-time key.</p>`)
		return
	}
	page, _ := staticFiles.ReadFile("static/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
	w.Write(page)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// ---------- state ----------

type stateJSON struct {
	Workspace *models.Workspace `json:"workspace"`
	History   []historyItem     `json:"history"`
	Info      Info              `json:"info"`
	Languages []langJSON        `json:"languages"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, stateJSON{Workspace: s.ws, History: historyItems(s.hist), Info: s.info, Languages: languages()})
}

// putWorkspace replaces the workspace with the page's copy and saves it.
func (s *Server) putWorkspace(w http.ResponseWriter, r *http.Request) {
	var ws models.Workspace
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&ws); err != nil {
		writeError(w, http.StatusBadRequest, "the workspace is not valid JSON: "+err.Error())
		return
	}
	if ws.Version == 0 {
		ws.Version = models.CurrentVersion
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.ws = ws
	if err := s.save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// save writes the workspace; s.mu must be held.
func (s *Server) save() error {
	if s.store == nil {
		return nil
	}
	if err := s.store.Save(s.ws); err != nil {
		return fmt.Errorf("could not save the workspace: %w", err)
	}
	return nil
}

func (s *Server) saveHistory() {
	if s.store != nil {
		_ = s.store.SaveHistory(s.hist)
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(v); err != nil {
		return fmt.Errorf("bad request: %w", err)
	}
	return nil
}

// Options configures Run.
type Options struct {
	// Port to listen on. 0 picks a free one. When the port is taken, Run
	// falls back to a free one and says so in Addresses.Fallback.
	Port int
	// LAN listens on every interface so other devices on the network can
	// open the GUI (they still need the link with its key). Off, only this
	// machine can.
	LAN  bool
	Info Info
	// Ready is called once the server listens.
	Ready func(Addresses)
}

// Addresses lists where the GUI can be opened. Every URL carries the key.
type Addresses struct {
	Local    []string // http://localhost:port/?t=..., http://127.0.0.1:port/?t=...
	Network  []string // one per LAN address, when Options.LAN is set
	Port     int
	Fallback string // why the requested port wasn't used, if it wasn't
}

// Open is the URL to open in this machine's browser.
func (a Addresses) Open() string { return a.Local[0] }

// listen tries the requested port, then any free one.
func listen(host string, port int) (net.Listener, string, error) {
	l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil || port == 0 {
		return l, "", err
	}
	l, err2 := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err2 != nil {
		return nil, "", err
	}
	return l, fmt.Sprintf("port %d is not available (%v), so a free port was picked", port, unwrapOp(err)), nil
}

func unwrapOp(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Err != nil {
		return op.Err
	}
	return err
}

// lanIPs lists this machine's non-loopback unicast addresses, IPv4 first.
func lanIPs() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var v4, v6 []net.IP
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() || n.IP.IsMulticast() || n.IP.IsUnspecified() {
			continue
		}
		if n.IP.To4() != nil {
			v4 = append(v4, n.IP)
		} else {
			v6 = append(v6, n.IP)
		}
	}
	return append(v4, v6...)
}

// Run serves the web UI until ctx is cancelled.
func Run(ctx context.Context, ws *models.Workspace, store *storage.Store, opts Options) error {
	if opts.Port < 0 || opts.Port > 65535 {
		return fmt.Errorf("bad port %d", opts.Port)
	}
	host := "127.0.0.1"
	if opts.LAN {
		host = "" // every interface, IPv4 and IPv6
	}
	l, fallback, err := listen(host, opts.Port)
	if err != nil {
		return err
	}
	port := l.Addr().(*net.TCPAddr).Port
	p := strconv.Itoa(port)
	s, err := New(ws, store, opts.Info, "http://127.0.0.1:"+p)
	if err != nil {
		l.Close()
		return err
	}
	addrs := Addresses{Port: port, Fallback: fallback}
	for _, h := range []string{"localhost", "127.0.0.1"} {
		addrs.Local = append(addrs.Local, "http://"+net.JoinHostPort(h, p)+"/?t="+s.token)
	}
	if opts.LAN {
		s.hosts["[::1]:"+p] = true
		for _, ip := range lanIPs() {
			hp := net.JoinHostPort(ip.String(), p)
			s.hosts[hp] = true
			addrs.Network = append(addrs.Network, "http://"+hp+"/?t="+s.token)
		}
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	if opts.Ready != nil {
		opts.Ready(addrs)
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := srv.Shutdown(shutdown)
		if errors.Is(err, context.DeadlineExceeded) {
			return nil // streams and sockets still open; we're exiting anyway
		}
		return err
	}
}
