package wsclient_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
	"github.com/AbyAbyss/cli-rest-client/pkg/wsclient"
)

func TestEchoAndClose(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ctx := context.Background()

	req, _ := http.NewRequest("GET", "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws?room=1", nil)
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Sec-WebSocket-Protocol", "echo")
	conn, hs, err := wsclient.Dial(ctx, req, wsclient.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if hs.Status != 101 || hs.Protocol != "echo" {
		t.Fatalf("handshake %+v", hs)
	}
	m, err := conn.Read(ctx)
	if err != nil || m.Sent || !strings.Contains(m.Data, `"authorization":"Bearer abc"`) || !strings.Contains(m.Data, `"query":"room=1"`) {
		t.Fatalf("welcome %+v %v", m, err)
	}
	sent, err := conn.Send(ctx, `{"hello":"world"}`)
	if err != nil || !sent.Sent || sent.Size != 17 {
		t.Fatalf("send %+v %v", sent, err)
	}
	if m, err = conn.Read(ctx); err != nil || m.Data != `{"hello":"world"}` {
		t.Fatalf("echo %+v %v", m, err)
	}
	conn.Send(ctx, "bye")
	_, err = conn.Read(ctx)
	if got := wsclient.Describe(err); got != "closed normally (1000): bye" {
		t.Fatalf("describe %q (%v)", got, err)
	}
}

func TestRefusedUpgrade(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	req, _ := http.NewRequest("GET", "ws"+strings.TrimPrefix(srv.URL, "http")+"/json", nil)
	if _, _, err := wsclient.Dial(context.Background(), req, wsclient.Options{}); err == nil || !strings.Contains(err.Error(), "refused the upgrade with 200 OK") {
		t.Fatalf("got %v", err)
	}
}

func TestDisconnect(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequest("GET", "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	conn, _, err := wsclient.Dial(ctx, req, wsclient.Options{})
	if err != nil {
		t.Fatal(err)
	}
	conn.Read(ctx) // welcome
	cancel()
	_, err = conn.Read(ctx)
	if got := wsclient.Describe(err); got != "disconnected" {
		t.Fatalf("describe %q (%v)", got, err)
	}
}
