// Package storage loads and saves the workspace file.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// EnvDataPath overrides the default workspace location when set.
const EnvDataPath = "TERM_REST_CLIENT_DATA"

// Store reads and writes a workspace JSON file.
type Store struct {
	Path string
}

// DefaultPath returns the workspace path: $TERM_REST_CLIENT_DATA if set,
// otherwise <user config dir>/term-rest-client/workspace.json.
func DefaultPath() (string, error) {
	if p := os.Getenv(EnvDataPath); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine config directory (set %s): %w", EnvDataPath, err)
	}
	return filepath.Join(dir, "term-rest-client", "workspace.json"), nil
}

// Load reads the workspace. A missing file yields the sample workspace and
// created=true; it is not written until Save is called.
func (s *Store) Load() (ws *models.Workspace, created bool, err error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return SampleWorkspace(), true, nil
	}
	if err != nil {
		return nil, false, err
	}
	ws = &models.Workspace{}
	if err := json.Unmarshal(data, ws); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	ws.Normalize()
	return ws, false, nil
}

// Save writes the workspace atomically (temp file + rename).
func (s *Store) Save(ws *models.Workspace) error {
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".workspace-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.Path)
}

// SampleWorkspace is what a first run starts with.
func SampleWorkspace() *models.Workspace {
	req := func(name, method, url, bodyType, body string) *models.Request {
		r := models.NewRequest(name)
		r.Method = method
		r.URL = url
		r.BodyType = bodyType
		r.Body = body
		return &r
	}

	login := req("Login", "GET", "{{baseUrl}}/basic-auth/user/passwd", models.BodyNone, "")
	login.Auth = models.Auth{Type: models.AuthBasic, Username: "user", Password: "passwd", In: "header"}
	login.Tests = "status == 200\njson.authenticated == true"

	bearer := req("Bearer Token", "GET", "{{baseUrl}}/bearer", models.BodyNone, "")
	bearer.Auth = models.Auth{Type: models.AuthBearer, Token: "{{token}}", In: "header"}
	bearer.Tests = "status == 200\njson.token == {{token}}"

	getAll := req("Get JSON", "GET", "{{baseUrl}}/json", models.BodyNone, "")
	getAll.Tests = "status == 200\nheader Content-Type contains json\njson.slideshow.title exists"

	search := req("Query Params", "GET", "{{baseUrl}}/get", models.BodyNone, "")
	search.Params = []models.KeyValue{{Key: "page", Value: "1"}, {Key: "q", Value: "terminal client"}}
	search.Headers = []models.KeyValue{{Key: "X-Request-Id", Value: "{{$uuid}}"}}
	search.Tests = "status == 200\njson.args.page == 1\njson.args.q == \"terminal client\""

	create := req("Create User", "POST", "{{baseUrl}}/post", models.BodyJSON, "{\n  \"name\": \"Aby\",\n  \"role\": \"admin\",\n  \"createdAt\": {{$timestamp}}\n}")
	create.Tests = "status == 200\n# httpbin echoes the body back under \"json\"\njson.json.name == Aby\nset lastUser = json.json.name"

	update := req("Update User", "PUT", "{{baseUrl}}/put", models.BodyJSON, "{\n  \"id\": 1,\n  \"active\": true\n}")
	update.Tests = "status == 200\njson.json.active == true"

	form := req("Form Login", "POST", "{{baseUrl}}/post", models.BodyForm, "username=aby\npassword=secret")
	form.Tests = "status == 200\njson.form.username == aby"

	remove := req("Delete User", "DELETE", "{{baseUrl}}/delete", models.BodyNone, "")
	remove.Tests = "status == 200"

	charge := req("Charge", "POST", "{{baseUrl}}/post", models.BodyJSON, "{\n  \"amount\": 200,\n  \"currency\": \"INR\",\n  \"orderId\": \"{{orderId}}\"\n}")
	charge.PreRequest = "# runs before the request is sent\nset orderId = order-{{$randomInt}}"
	charge.Tests = "status == 200\ntime < 5000\njson.json.orderId == {{orderId}}"

	health := req("Health Check", "GET", "{{baseUrl}}/get", models.BodyNone, "")
	health.Tests = "status == 200\ntime < 2000"

	ws := &models.Workspace{
		Version: models.CurrentVersion,
		Collections: []*models.Collection{
			{Name: "Auth API", Requests: []*models.Request{login, bearer}},
			{
				Name: "User Service",
				Folders: []*models.Collection{
					{
						Name:     "Users",
						Folders:  []*models.Collection{{Name: "Admin", Requests: []*models.Request{remove}}},
						Requests: []*models.Request{create, update},
					},
					{Name: "Lookup", Requests: []*models.Request{getAll, search}},
				},
				Requests: []*models.Request{form},
			},
			{
				Name:    "Payment Gateway",
				Folders: []*models.Collection{{Name: "Charges", Requests: []*models.Request{charge}}},
			},
		},
		Requests: []*models.Request{health},
		Variables: []models.KeyValue{
			{Key: "baseUrl", Value: "https://httpbin.org"},
			{Key: "token", Value: "my-secret-token"},
		},
		Settings: models.Settings{Theme: "Catppuccin Mocha", TimeoutSeconds: 30},
	}
	ws.Normalize()
	return ws
}
