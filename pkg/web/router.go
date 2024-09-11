package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type PupRouter struct {
	pm dogeboxd.PupManager
}

func (t PupRouter) RouteRequest(w http.ResponseWriter, r *http.Request) {
	// Who is this request from?
	var originIsPup bool = false
	var originIP string

	// handle proxies
	if r.Header.Get("X-Forwarded-For") != "" {
		// If there are multiple IPs in X-Forwarded-For, take the first one
		originIP = strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]
	} else {
		// otherwise just use the remote address
		originIP = strings.Split(r.RemoteAddr, ":")[0]
	}

	originPup, _, err := t.pm.FindPupByIP(originIP)
	if err == nil {
		originIsPup = true
	}

	// Who is this request going to?
	path := strings.TrimRight(r.URL.Path, "/") // unsure if we want to trim ...
	pathSegments := strings.Split(path, "/")
	iface := pathSegments[0]        // first part of the path is the target interface
	pathSegments = pathSegments[1:] // trim that out of the request

	// Handle dbx requests:
	if iface == "dbx" {
		// TODO
		forbidden(w, "dbx apis currently unavailable")
		return
	}

	// Handle interface requests:
	if !originIsPup {
		// you must be a pup!
		forbidden(w, "You are not a Pup we know about")
		return

	}
	// check the pup has a provider for this interface and get the provider pup
	providerID, ok := originPup.Providers[iface]
	if !ok {
		forbidden(w, "Your manifest does not depend on interface: ", iface)
		return
	}

	providerPup, _, err := t.pm.GetPup(providerID)
	if err != nil {
		forbidden(w, "Your pup's provider for this interface no longer exists")
		return
	}

	// Does the request match any of their permissionGroup routes?
	routes := []string{}
	for _, i := range originPup.Manifest.Interfaces {
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
		routeSegments := strings.Split(route, "/")

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

	// Serve the request to the proxy
	proxy.ServeHTTP(w, proxyReq)
}

func forbidden(w http.ResponseWriter, reasons ...string) {
	reason := "Access Denied"
	if len(reasons) > 0 {
		reason = fmt.Sprint(reasons)
	}
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(reason))
}
