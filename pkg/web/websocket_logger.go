package web

import (
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"golang.org/x/net/websocket"
)

func GetLogHandler(PupID string, dbx dogeboxd.Dogeboxd) (*websocket.Server, error) {
	cancel, logChan, err := dbx.GetLogChannel(PupID)
	if err != nil {
		fmt.Println("ERR", err)
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

	// create a pump that broadcasts logs
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
					fmt.Println("ERR sending, closing websocket", err)
					conn.Close()
				}
			}
		}
	}()

	return &h, nil
}
