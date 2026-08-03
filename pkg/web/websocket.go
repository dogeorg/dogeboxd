package web

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"golang.org/x/net/websocket"
)

// Represents a websocket connection from a client
type WSCONN struct {
	WS        *websocket.Conn
	Stop      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
}

func (t *WSCONN) IsClosed() bool {
	return t.closed.Load()
}

func (t *WSCONN) Close() {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		close(t.Stop)
		if t.WS != nil {
			_ = t.WS.Close()
		}
	})
}

// Handle incomming websocket connections for general updates
func (t api) getUpdateSocket(w http.ResponseWriter, r *http.Request) {
	initialPayload := func() any {
		return dogeboxd.Change{ID: "internal", Error: "", Type: dogeboxd.ChangeTypeBootstrap, Update: t.getRawBS()}
	}
	t.ws.GetWSHandler(initialPayload, websocketRevoked(r, t.sm)).ServeHTTP(w, r)
}

// Handle incoming websocket connections for pup log output
func (t api) getPupLogSocket(w http.ResponseWriter, r *http.Request) {
	t.getLogSocket(w, r, "PupID", func(logID string, resumeToken *string, revoked <-chan struct{}) (*websocket.Server, error) {
		return GetLogHandler(logID, resumeToken, t.dbx, revoked)
	}, func(err error) string {
		return "Error establishing pup log channel"
	})
}

// Handle incoming websocket connections for job log output
func (t api) getJobLogSocket(w http.ResponseWriter, r *http.Request) {
	t.getLogSocket(w, r, "JobID", func(logID string, resumeToken *string, revoked <-chan struct{}) (*websocket.Server, error) {
		return GetJobLogHandler(logID, resumeToken, t.dbx, revoked)
	}, func(err error) string {
		return "Error establishing job log channel: " + err.Error()
	})
}

func (t api) getLogSocket(
	w http.ResponseWriter,
	r *http.Request,
	pathValue string,
	getHandler func(string, *string, <-chan struct{}) (*websocket.Server, error),
	getErrorMessage func(error) string,
) {
	logID := r.PathValue(pathValue)
	resumeToken := parseLogResumeToken(r)
	wh, err := getHandler(logID, resumeToken, websocketRevoked(r, t.sm))
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, getErrorMessage(err))
		return
	}
	wh.ServeHTTP(w, r)
}

// Handle incoming websocket connections for job updates
func (t api) getJobsSocket(w http.ResponseWriter, r *http.Request) {
	wh := t.GetJobsHandler(websocketRevoked(r, t.sm))
	wh.ServeHTTP(w, r)
}

func websocketRevoked(r *http.Request, sm dogeboxd.StateManager) <-chan struct{} {
	if session, ok := sessionFromContext(r.Context()); ok {
		return session.Done()
	}

	// ConfiguredAuth admits the setup state channel without a session while the
	// system is unconfigured. Close that connection as soon as setup completes.
	revoked := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		defer close(revoked)
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if sm.Get().Dogebox.InitialState.HasFullyConfigured {
					return
				}
			}
		}
	}()
	return revoked
}

func parseLogResumeToken(r *http.Request) *string {
	rawResumeToken := r.URL.Query().Get("resumeToken")
	if rawResumeToken == "" {
		return nil
	}

	return &rawResumeToken
}
