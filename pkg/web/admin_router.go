package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
)

func NewAdminRouter(config dogeboxd.ServerConfig, pm dogeboxd.PupManager) conductor.Service {
	return AdminRouter{
		config: config,
		pm:     pm,
		prx:    map[string]*adminProxy{},
	}
}

type AdminRouter struct {
	config dogeboxd.ServerConfig
	pm     dogeboxd.PupManager
	prx    map[string]*adminProxy
}

func (t *AdminRouter) updateProxies() {
	// find pups that have admin ports
	visited := map[string]bool{}
	for pupid, pup := range t.pm.GetStateMap() {
		for _, ui := range pup.WebUIs {
			id := fmt.Sprintf("%s:%s", pupid, ui.Port)
			visited[id] = true
			_, exists := t.prx[id]
			if !exists {
				t.prx[id] = &adminProxy{
					bindPort: ui.Port,
					destHost: pup.IP,
					destPort: ui.Internal,
				}
				t.prx[id].Start()
			}
		}
	}
	// close any that no longer exist
	for id := range t.prx {
		_, exists := visited[id]
		if !exists {
			t.prx[id].Stop()
			delete(t.prx, id)
		}
	}
}

func (t AdminRouter) Run(started, stopped chan bool, stop chan context.Context) error {
	t.updateProxies()
	go func() {
		go func() {
		mainloop:
			for {
				select {
				case <-stop:
					break mainloop
				case p, ok := <-t.pm.GetUpdateChannel():
					if !ok {
						break mainloop
					}
					if p.Event == dogeboxd.PUP_ADOPTED {
						// New pup adopted, update proxies
						t.updateProxies()
					}
				}
			}
		}()

		started <- true
		<-stop
		// if we're stopping, shut down all the admin proxies
		for _, prx := range t.prx {
			prx.Stop()
		}
		stopped <- true
	}()
	return nil
}

// adminProxy does the actual proxying, AdminRouter manages these proxies.
type adminProxy struct {
	bindPort int
	destHost string
	destPort int
	stop     context.CancelFunc
}

func (t *adminProxy) Start() {
	fmt.Printf("Starting admin proxy %d -> %d\n", t.bindPort, t.destPort)
	ctx, cancel := context.WithCancel(context.Background())
	t.stop = cancel

	target := fmt.Sprintf("http://%s:%d", t.destHost, t.destPort)
	proxyURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Failed to parse URL: %v", err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", t.bindPort),
		Handler: httputil.NewSingleHostReverseProxy(proxyURL),
	}

	// handle stopping
	go func() {
		<-ctx.Done()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Fatalf("Could not gracefully shutdown admin proxy: %v", err)
		}
	}()

	// Start the server
	go func() {
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server ListenAndServe: %v", err)
		}
	}()
}

func (t *adminProxy) Stop() {
	t.stop()
}
