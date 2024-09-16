package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func sendResponse(w http.ResponseWriter, payload any) {
	// note: w.Header after this, so we can call sendError
	b, err := json.Marshal(payload)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("in json.Marshal: %s", err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.Write(b)
}

func sendErrorResponse(w http.ResponseWriter, code int, message string) {
	log.Printf("[!] %d: %s\n", code, message)
	// would prefer to use json.Marshal, but this avoids the need
	// to handle encoding errors arising from json.Marshal itself!
	payload := fmt.Sprintf("{\"error\":{\"code\":%q,\"message\":%q}}", code, message)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.WriteHeader(code)
	w.Write([]byte(payload))
}

func getOriginIP(r *http.Request) string {
	var originIP string

	// handle proxies
	if r.Header.Get("X-Forwarded-For") != "" {
		// If there are multiple IPs in X-Forwarded-For, take the first one
		originIP = strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]
	} else {
		// otherwise just use the remote address
		originIP = strings.Split(r.RemoteAddr, ":")[0]
	}

	return originIP
}
