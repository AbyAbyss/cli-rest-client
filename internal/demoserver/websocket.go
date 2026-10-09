package demoserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
)

// addWebSocket registers /ws, an echo server. On connect it sends
//
//	{"type":"welcome","authorization":"<the handshake's Authorization header>","query":"<raw query>"}
//
// then echoes every message back as it came. The message "bye" makes the
// server close the connection normally with the reason "bye".
func addWebSocket(mux *http.ServeMux) {
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"echo"}})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := context.Background()
		welcome, _ := json.Marshal(map[string]string{"type": "welcome", "authorization": r.Header.Get("Authorization"), "query": r.URL.RawQuery})
		if c.Write(ctx, websocket.MessageText, welcome) != nil {
			return
		}
		for {
			typ, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageText && string(data) == "bye" {
				c.Close(websocket.StatusNormalClosure, "bye")
				return
			}
			if c.Write(ctx, typ, data) != nil {
				return
			}
		}
	})
}
