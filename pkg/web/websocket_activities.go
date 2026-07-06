package web

import (
	"fmt"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"golang.org/x/net/websocket"
)

// GetJobsHandler creates a WebSocket handler for real-time job updates
// Sends initial jobs list, then streams job:created, job:updated, job:completed, job:failed events
func (t api) GetJobsHandler() *websocket.Server {
	initialPayload := func() any {
		if _, err := t.dbx.DetectAndMarkOrphanedJobs(); err != nil {
			fmt.Printf("failed to detect orphaned jobs during bootstrap: %v\n", err)
		}

		// Load all jobs from database as initial payload
		jobs, err := t.dbx.JobManager.GetAllJobs()
		if err != nil {
			return dogeboxd.Change{
				ID:     "internal",
				Type:   "bootstrap",
				Update: map[string]interface{}{"jobs": []dogeboxd.JobRecord{}},
			}
		}

		return dogeboxd.Change{
			ID:     "internal",
			Type:   "bootstrap",
			Update: map[string]interface{}{"jobs": jobs},
		}
	}

	return t.ws.GetWSHandler(initialPayload)
}

// GetJobLogHandler creates a WebSocket handler for streaming job logs
// Uses the same log streaming mechanism as pup logs (ActionLogger)
func GetJobLogHandler(JobID string, resumeToken *string, dbx dogeboxd.Dogeboxd) (*websocket.Server, error) {
	// Get log channel for this job (same system as pup logs)
	cancel, logChan, err := dbx.GetJobLogChannel(JobID, resumeToken)
	if err != nil {
		return nil, err
	}

	config := &websocket.Config{
		Origin: nil,
	}

	stop := make(chan bool)  // WSCONN stop channel
	start := make(chan bool) // tell the goroutine pump to start
	conn := WSCONN{Stop: stop}

	h := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			conn.WS = ws
			start <- true
			<-stop   // hold the connection until stopper closes
			cancel() // tell the log producer to stop
		},
		Config: *config,
	}

	// Create a pump that broadcasts logs (same as pup log streaming)
	go func() {
		<-start
	out:
		for {
			select {
			case <-stop:
				break out
			case v, ok := <-logChan:
				if !ok {
					conn.Close()
					break
				}
				err := websocket.JSON.Send(conn.WS, v)
				if err != nil {
					fmt.Println("ERR sending job log, closing websocket", err)
					conn.Close()
				}
			}
		}
	}()

	return &h, nil
}
