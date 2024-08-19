package dogeboxd

import (
	"context"
	"fmt"
	"log"
	"sync"

	"golang.org/x/net/websocket"
)

const WS_DEFAULT_CHANNEL string = "updates"

type WSRelay struct {
	config ServerConfig
	socks  []WSCONN
	relay  chan Change
	newWs  chan WSCONN
}

func NewWSRelay(config ServerConfig, relay chan Change) WSRelay {
	if config.Recovery {
		log.Printf("In recovery mode: not initialising WSRelay")
		return WSRelay{}
	}

	return WSRelay{
		config: config,
		socks:  []WSCONN{},
		relay:  relay,
		newWs:  make(chan WSCONN),
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
					t.Broadcast(WS_DEFAULT_CHANNEL, v)
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

func (t *WSRelay) Broadcast(channel string, v any) {
	// fmt.Println(len(t.socks), "BCAST", channel, ":::", v)

	var deleteMe []int
	for i, ws := range t.socks {
		if ws.channel != channel {
			continue
		}
		fmt.Println("sending to sock", i)
		err := websocket.JSON.Send(ws.WS, v)
		if err != nil {
			fmt.Println("ERR WS", err)
			deleteMe = append(deleteMe, i)
		}
	}
	if len(deleteMe) > 0 {
		for pos := range deleteMe {
			t.socks[pos].Close()
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

func (t WSRelay) GetWSHandler(channel string, initialPayloader func() any) *websocket.Server {
	config := &websocket.Config{
		Origin: nil,
	}
	h := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			fmt.Println("HANDL")
			stop := make(chan bool)
			t.newWs <- WSCONN{ws, stop, sync.Once{}, channel}

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

func (t *WSRelay) GetWSChannelHandler(channel string, ch chan string, cancel context.CancelFunc) *websocket.Server {
	config := &websocket.Config{
		Origin: nil,
	}

	stop := make(chan bool)
	start := make(chan bool)
	h := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			fmt.Println("HANDL")
			t.newWs <- WSCONN{ws, stop, sync.Once{}, channel}
			start <- true
			<-stop // hold the connection until stopper closes
			// cancel the producer
			cancel()
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
			case s, ok := <-ch:
				if !ok {
					close(stop)
					break
				}
				fmt.Println(">>", channel, s)
				t.Broadcast(channel, s)
			}
		}
	}()
	return &h
}

type WSCONN struct {
	WS      *websocket.Conn
	Stop    chan bool
	once    sync.Once
	channel string // 'channel' discriminator for messages
}

func (t *WSCONN) Close() {
	t.once.Do(func() {
		close(t.Stop)
	})
}
