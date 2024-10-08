package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
)

func NewInternalRouter(config dogeboxd.ServerConfig, dbx dogeboxd.Dogeboxd, pm dogeboxd.PupManager, dkm dogeboxd.DKMManager) conductor.Service {
	return InternalRouter{
		config: config,
		pm:     pm,
		dbx:    dbx,
		dbxmux: http.NewServeMux(),
		dkm:    dkm,
	}
}

type InternalRouter struct {
	config dogeboxd.ServerConfig
	dbx    dogeboxd.Dogeboxd
	pm     dogeboxd.PupManager
	dkm    dogeboxd.DKMManager
	dbxmux *http.ServeMux
}

func (t InternalRouter) routes() {
	t.dbxmux.HandleFunc("POST /dbx/metrics", t.recordMetrics)
	t.dbxmux.HandleFunc("/dbx/hook/{hookID}", t.hookHandler)
	// TODO: this api needs rethinking
	// t.dbxmux.HandleFunc("POST /dbx/keys/getDelegatedKeys", t.getDelegatedPupKeys)
}

func (t InternalRouter) Run(started, stopped chan bool, stop chan context.Context) error {
	t.routes()
	go func() {
		retry := time.NewTimer(time.Second)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", "10.69.0.1", t.config.InternalPort), Handler: t}
		go func() {
		mainloop:
			for {
				select {
				case <-stop:
					retry.Stop()
					break mainloop
				case <-retry.C:
					// check if we have any installed pups
					runRouter := false
					for _, p := range t.pm.GetStateMap() {
						if p.Installation == dogeboxd.STATE_READY {
							runRouter = true
							break
						}
					}
					if runRouter {
						fmt.Println("connecting internal router")
						if err := srv.ListenAndServe(); err != http.ErrServerClosed {
							//
						}
					}
					retry.Reset(time.Second)
				}
			}
		}()

		started <- true
		ctx := <-stop
		srv.Shutdown(ctx)
		stopped <- true
	}()
	return nil
}

func (t InternalRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Who is this request going to?
	path := strings.TrimRight(r.URL.Path, "/") // unsure if we want to trim ...
	pathSegments := strings.Split(path, "/")
	iface := pathSegments[1]        // first part of the path is the target interface
	pathSegments = pathSegments[2:] // trim that out of the request

	// Handle dbx requests:
	if iface == "dbx" {
		t.dbxmux.ServeHTTP(w, r)
		return
	}

	// Who is this request from?
	originPup, ok := t.getOriginPup(r)
	if !ok {
		// you must be a pup!
		forbidden(w, "You are not a Pup we know about")
		return
	}
	//
	// check the pup has a provider for this interface and get the provider pup
	providerID, ok := originPup.Providers[iface]
	if !ok {
		forbidden(w, "Your pup has no provider for interface: ", iface)
		return
	}

	providerPup, _, err := t.pm.GetPup(providerID)
	if err != nil {
		forbidden(w, "Your pup's provider for this interface no longer exists")
		return
	}

	// Does the request match any of their permissionGroup routes the provider provides?
	routes := []string{}
	for _, i := range providerPup.Manifest.Interfaces {
		if i.Name == iface {
			for _, pg := range i.PermissionGroups {
				for _, r := range pg.Routes {
					routes = append(routes, r)
				}
			}
		}
	}

	matchingRoute := ""
	for _, route := range routes {
		// route = strings.Split(route, "/")
		routeSegments := strings.Split(route, "/")[1:]
		// Check if the route and path have the same number of segments or if the route has one less (due to a wildcard)
		if len(routeSegments) > len(pathSegments) || len(routeSegments) == len(pathSegments)-1 {
			continue
		}

		wildcardFound := false
		match := true

		for i, segment := range routeSegments {
			if segment == "*" {
				if wildcardFound {
					// More than one wildcard is not allowed
					match = false
					break
				}
				wildcardFound = true
			} else if i >= len(pathSegments) || segment != pathSegments[i] {
				match = false
				break
			}
		}

		if match {
			matchingRoute = route
			break
		}
	}

	if matchingRoute == "" {
		forbidden(w, "No matching route available")
		return
	}

	// Rewrite the request and proxy
	host := providerPup.IP
	port := 0
	// find the port the interface is listening on
	for _, exp := range providerPup.Manifest.Container.Exposes {
		for _, i := range exp.Interfaces {
			if iface == i {
				port = exp.Port
			}
		}
	}
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d/%s", host, port, strings.Join(pathSegments, "/")))
	if err != nil {
		http.Error(w, "Failed to parse URL", http.StatusInternalServerError)
		return
	}

	// Copy the original request's query parameters
	targetURL.RawQuery = r.URL.RawQuery

	// Create a new request with the modified URL
	proxyReq := new(http.Request)
	*proxyReq = *r
	proxyReq.URL = targetURL
	proxyReq.Host = targetURL.Host

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	fmt.Printf("[router: %s -> %s] %s\n", originPup.Manifest.Meta.Name, providerPup.Manifest.Meta.Name, targetURL.Path)
	// Serve the request to the proxy
	proxy.ServeHTTP(w, proxyReq)
}

func forbidden(w http.ResponseWriter, reasons ...string) {
	reason := "Access Denied"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, " ")
	}
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(reason))
}

func (t InternalRouter) getOriginPup(r *http.Request) (dogeboxd.PupState, bool) {
	var originIsPup bool = false
	originIP := getOriginIP(r)
	originPup, _, err := t.pm.FindPupByIP(originIP)
	if err == nil {
		originIsPup = true
	}
	return originPup, originIsPup
}
