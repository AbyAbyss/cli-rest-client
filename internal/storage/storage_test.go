package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ws.json")
	s := &Store{Path: path}

	ws, created, err := s.Load()
	if err != nil || !created || len(ws.Collections) == 0 {
		t.Fatalf("missing file should give the sample workspace: %v %v", created, err)
	}
	folders, index, _ := ws.Location(ws.Collections[2].Folders[0].Requests[0])
	ws.Draft = &models.Draft{Request: *ws.Collections[2].Folders[0].Requests[0], Folders: folders, Index: index}
	if err := s.Save(ws); err != nil {
		t.Fatal(err)
	}
	got, created, err := s.Load()
	if err != nil || created {
		t.Fatalf("reload: %v %v", created, err)
	}
	if len(got.Collections) != len(ws.Collections) || !got.Collections[0].Requests[0].Equal(*ws.Collections[0].Requests[0]) {
		t.Fatalf("round trip mismatch")
	}
	if got.Draft == nil || got.Draft.Request.Name != ws.Draft.Request.Name || got.AtLocation(got.Draft.Folders, got.Draft.Index).Name != "Charge" {
		t.Fatalf("draft lost")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestLoadOldFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ws.json")
	// Minimal file without newer fields.
	os.WriteFile(path, []byte(`{"collections":[{"name":"C","requests":[{"name":"R","url":"http://x","body":"{}"}, null]}, null]}`), 0o644)
	ws, _, err := (&Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	r := ws.Collections[0].Requests[0]
	if len(ws.Collections) != 1 || len(ws.Collections[0].Requests) != 1 || r.Method != "GET" || r.BodyType != models.BodyJSON || r.Auth.Type != models.AuthNone {
		t.Fatalf("defaults not applied: %+v", r)
	}
	if ws.Settings.TimeoutSeconds != 30 {
		t.Fatalf("timeout default not applied")
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ws.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, _, err := (&Store{Path: path}).Load(); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDefaultPathEnv(t *testing.T) {
	t.Setenv(EnvDataPath, "/tmp/custom.json")
	if p, _ := DefaultPath(); p != "/tmp/custom.json" {
		t.Fatalf("got %s", p)
	}
}
