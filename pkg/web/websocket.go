package web

import (
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"golang.org/x/net/websocket"
)

// Represents a websocket connection from a client
type WSCONN struct {
	WS   *websocket.Conn
	Stop chan bool
}

func (t *WSCONN) IsClosed() bool {
	return t.Stop == nil
}

func (t *WSCONN) Close() {
	if t.Stop != nil {
		close(t.Stop)
		t.Stop = nil
	}
}

// Handle incomming websocket connections for general updates
func (t api) getUpdateSocket(w http.ResponseWriter, r *http.Request) {
	initialPayload := func() any {
		return dogeboxd.Change{ID: "internal", Error: "", Type: "bootstrap", Update: t.getRawBS()}
	}
	t.ws.GetWSHandler(initialPayload).ServeHTTP(w, r)
}

// Handle incomming websocket connections for log output
func (t api) getLogSocket(w http.ResponseWriter, r *http.Request) {
	PupID := r.PathValue("PupID")
	wh, err := GetLogHandler(PupID, t.dbx)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error establishing log channel")
		return
	}
	wh.ServeHTTP(w, r)
}
