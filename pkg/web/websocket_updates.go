package web

import (
	"context"
	"fmt"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"golang.org/x/net/websocket"
)

type WSRelay struct {
	config dogeboxd.ServerConfig
	socks  []*WSCONN
	relay  chan dogeboxd.Change
	newWs  chan *WSCONN
}

func NewWSRelay(config dogeboxd.ServerConfig, relay chan dogeboxd.Change) WSRelay {
	return WSRelay{
		config: config,
		socks:  []*WSCONN{},        // all current connections
		relay:  relay,              // recieve Change messages from Dogeboxd to broadcast
		newWs:  make(chan *WSCONN), // recieve new WSCONNs
	}
}

func (t WSRelay) Run(started, stopped chan bool, stop chan context.Context) error {
	cleanupTime := 10 * time.Second
	cleanup := time.NewTimer(cleanupTime)
	go func() {
		go func() {
		mainloop:
			for {
				select {
				case <-stop:
					break mainloop
				case ws := <-t.newWs:
					t.addSock(ws)
				case v := <-t.relay:
					t.broadcast(v)
				case <-cleanup.C:
					t.cleanupSocks()
					cleanup.Reset(cleanupTime)
				}
			}
		}()

		started <- true
		<-stop
		for _, sock := range t.socks {
			sock.Close()
		}
		stopped <- true
	}()
	return nil
}

func (t *WSRelay) cleanupSocks() {
	remaining := []*WSCONN{}
	for _, s := range t.socks {
		if s.IsClosed() {
			continue
		}
		remaining = append(remaining, s)
	}
	t.socks = remaining
}

func (t *WSRelay) broadcast(v any) {
	for _, ws := range t.socks {
		if ws.IsClosed() {
			continue
		}
		err := websocket.JSON.Send(ws.WS, v)
		if err != nil {
			ws.Close()
		}
	}
}

func (t *WSRelay) addSock(ws *WSCONN) {
	t.socks = append(t.socks, ws)
}

func (t WSRelay) GetWSHandler(initialPayloader func() any) *websocket.Server {
	config := &websocket.Config{
		Origin: nil,
	}
	h := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			stop := make(chan bool)
			t.newWs <- &WSCONN{ws, stop}

			err := websocket.JSON.Send(ws, initialPayloader())
			if err != nil {
				fmt.Println("failed to send initial payload", err)
			}
			<-stop // hold the connection until stopper closes
		},
		Config: *config,
	}
	return &h
}
