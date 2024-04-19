package dogeboxd

import (
	"context"
	"fmt"

	"golang.org/x/net/websocket"
)

type WSRelay struct {
	socks []WSCONN
	relay chan Change
	newWs chan WSCONN
}

func NewWSRelay(relay chan Change) WSRelay {
	return WSRelay{
		socks: []WSCONN{},
		relay: relay,
		newWs: make(chan WSCONN),
	}
}

func (t WSRelay) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
				select {
				case <-stop:
					break mainloop
				case ws := <-t.newWs:
					t.AddSock(ws)
				case v := <-t.relay:
					t.Broadcast(v)
				}
			}
		}()

		started <- true
		<-stop
		for _, sock := range t.socks {
			close(sock.Stop)
		}
		stopped <- true
	}()
	return nil
}

func (t *WSRelay) Broadcast(v any) {
	fmt.Println(len(t.socks))
	var deleteMe []int
	for i, ws := range t.socks {
		fmt.Println("sending to sock", i)
		err := websocket.JSON.Send(ws.WS, v)
		if err != nil {
			fmt.Println("ERR WS", err)
			deleteMe = append(deleteMe, i)
		}
	}
	if len(deleteMe) > 0 {
		for pos := range deleteMe {
			close(t.socks[pos].Stop)
			fmt.Println("removing sock", pos)
			t.socks[pos] = t.socks[len(t.socks)-1]
		}
		t.socks = t.socks[:len(t.socks)-len(deleteMe)]
	}
}

func (t *WSRelay) AddSock(ws WSCONN) {
	fmt.Println("Accepting new WS conn", ws)
	t.socks = append(t.socks, ws)
	fmt.Println(len(t.socks))
}

func (t WSRelay) GetWSHandler() *websocket.Server {
	config := &websocket.Config{
		Origin: nil,
	}
	h := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			fmt.Println("HANDL")
			stop := make(chan bool)
			t.newWs <- WSCONN{ws, stop}
			<-stop // hold the connection until stopper closes
		},
		Config: *config,
	}
	return &h
}

type WSCONN struct {
	WS   *websocket.Conn
	Stop chan bool
}
