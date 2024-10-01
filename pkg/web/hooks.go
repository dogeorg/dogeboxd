package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func (t InternalRouter) hookHandler(w http.ResponseWriter, r *http.Request) {
	var originIsPup bool = false
	originIP := getOriginIP(r)
	_, _, err := t.pm.FindPupByIP(originIP)
	if err == nil {
		originIsPup = true
	}

	if !originIsPup {
		// you must be a pup!
		forbidden(w, "You are not a Pup we know about", originIP)
		return
	}

	hookID := r.PathValue("hookID")
	handled := false
done:
	for _, pup := range t.pm.GetStateMap() {
		for _, hook := range pup.Hooks {
			if hookID == hook.ID {
				// check if the port is exposed first..
				exposed := false
				for _, ex := range pup.Manifest.Container.Exposes {
					if ex.Port == hook.Port {
						exposed = true
					}
				}
				if !exposed {
					sendErrorResponse(w, http.StatusUnauthorized, "Proxy to unexposed port unsupported")
					handled = true
					break done
				}

				// proxy the request
				targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d%s", pup.IP, hook.Port, hook.Path))
				if err != nil {
					sendErrorResponse(w, http.StatusInternalServerError, "couldn't compose proxy URL")
					handled = true
					break done
				}
				proxy := httputil.NewSingleHostReverseProxy(targetURL)
				proxy.Director = func(req *http.Request) {
					req.URL.Scheme = targetURL.Scheme
					req.URL.Host = targetURL.Host
					req.Header = r.Header
				}
				proxy.ServeHTTP(w, r)

				handled = true
				break done
			}
		}
	}

	if !handled {
		sendErrorResponse(w, http.StatusNotFound, "Hook not found")
	}
}
