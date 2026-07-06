package web

import (
	"net/http"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
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

// Handle incoming websocket connections for pup log output
func (t api) getPupLogSocket(w http.ResponseWriter, r *http.Request) {
	t.getLogSocket(w, r, "PupID", func(logID string, resumeToken *string) (*websocket.Server, error) {
		return GetLogHandler(logID, resumeToken, t.dbx)
	}, func(err error) string {
		return "Error establishing pup log channel"
	})
}

// Handle incoming websocket connections for job log output
func (t api) getJobLogSocket(w http.ResponseWriter, r *http.Request) {
	t.getLogSocket(w, r, "JobID", func(logID string, resumeToken *string) (*websocket.Server, error) {
		return GetJobLogHandler(logID, resumeToken, t.dbx)
	}, func(err error) string {
		return "Error establishing job log channel: " + err.Error()
	})
}

func (t api) getLogSocket(
	w http.ResponseWriter,
	r *http.Request,
	pathValue string,
	getHandler func(string, *string) (*websocket.Server, error),
	getErrorMessage func(error) string,
) {
	logID := r.PathValue(pathValue)
	resumeToken := parseLogResumeToken(r)
	wh, err := getHandler(logID, resumeToken)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, getErrorMessage(err))
		return
	}
	wh.ServeHTTP(w, r)
}

// Handle incoming websocket connections for job updates
func (t api) getJobsSocket(w http.ResponseWriter, r *http.Request) {
	wh := t.GetJobsHandler()
	wh.ServeHTTP(w, r)
}

func parseLogResumeToken(r *http.Request) *string {
	rawResumeToken := r.URL.Query().Get("resumeToken")
	if rawResumeToken == "" {
		return nil
	}

	return &rawResumeToken
}
