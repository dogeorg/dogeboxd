package web

import (
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/gorilla/securecookie"
)

const sessionExpiry = time.Hour

type Session struct {
	Token      string
	Expiration time.Time
	DKM_TOKEN  string
}

var sessions []Session

func getBearerToken(r *http.Request) (bool, string) {
	authHeader := r.Header.Get("authorization")

	if authHeader == "" {
		return false, ""
	}

	authPart := strings.Split(authHeader, " ")

	if len(authPart) != 2 {
		return false, ""
	}

	return true, authPart[1]
}

func getQueryToken(r *http.Request) (bool, string) {
	token := r.URL.Query().Get("token")
	if token == "" {
		return false, ""
	}
	return true, token
}

func getSession(r *http.Request, tokenExtractor func(r *http.Request) (bool, string)) (Session, bool) {
	tokenOK, token := tokenExtractor(r)
	if !tokenOK || token == "" {
		return Session{}, false
	}

	for i, session := range sessions {
		if session.Token == token {

			if time.Now().After(session.Expiration) {
				// Expired.
				sessions = append(sessions[:i], sessions[i+1:]...)
				return Session{}, false
			}

			return session, true
		}
	}

	return Session{}, false
}

func storeSession(session Session, config dogeboxd.ServerConfig) {
	sessions = append(sessions, session)

	if config.DevMode {
		file, err := os.OpenFile(fmt.Sprintf("%s/dev-sessions.gob", config.DataDir), os.O_RDWR|os.O_CREATE, 0666)
		if err == nil {
			encoder := gob.NewEncoder(file)
			err = encoder.Encode(sessions)
			if err != nil {
				log.Printf("Failed to encode sessions to dev-sessions.gob: %v", err)
			}
			file.Close()
		} else {
			log.Printf("Failed to open dev-sessions.gob: %v, ignoring..", err)
		}
	}
}

func newSession() (string, Session) {
	tokenBytes := securecookie.GenerateRandomKey(32)
	tokenHex := make([]byte, hex.EncodedLen(len(tokenBytes)))
	hex.Encode(tokenHex, tokenBytes)
	token := string(tokenHex)
	session := Session{
		Token:      token,
		Expiration: time.Now().Add(sessionExpiry),
	}
	return token, session
}

func delSession(r *http.Request) error {
	tokenOK, token := getBearerToken(r)
	if !tokenOK || token == "" {
		return errors.New("failed to fetch bearer token")
	}

	for i, session := range sessions {
		if session.Token == token {
			sessions = append(sessions[:i], sessions[i+1:]...)
			return nil
		}
	}

	return nil
}

func authReq(dbx dogeboxd.Dogeboxd, sm dogeboxd.StateManager, route string, next http.HandlerFunc) http.HandlerFunc {
	if route == "POST /authenticate" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	tokenExtractor := getBearerToken

	sessionHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := getSession(r, tokenExtractor)

		if !ok {
			w.WriteHeader(401)
			return
		}

		next.ServeHTTP(w, r)
	})

	// Helper function to handle system configuration check and authentication
	handleConfigCheck := func(w http.ResponseWriter, r *http.Request) {
		dbxis := sm.Get().Dogebox.InitialState

		if !dbxis.HasFullyConfigured {
			// We good.
			next.ServeHTTP(w, r)
			return
		}

		// Still check authentication if system is configured
		sessionHandler.ServeHTTP(w, r)
	}

	// Handle Websocket request authentication separately.
	if strings.HasPrefix(route, "/ws/") {
		tokenExtractor = getQueryToken
	}

	// We don't want a few routes to be locked down until the user has actually configured their system.
	// Whitelist those here.
	// TODO: Don't hardcode these.
	if route == "GET /system/bootstrap" ||
		route == "GET /system/recovery-bootstrap" ||
		route == "POST /system/bootstrap" ||
		route == "GET /system/disks" ||
		route == "GET /system/keymaps" ||
		route == "POST /system/keymap" ||
		route == "POST /system/hostname" ||
		route == "POST /system/storage" ||
		route == "POST /system/install" ||
		route == "GET /system/network/list" ||
		route == "PUT /system/network/set-pending" ||
		route == "GET /keys" ||
		route == "POST /keys/create-master" ||
		route == "POST /system/host/shutdown" ||
		route == "POST /system/host/reboot" ||
		route == "/ws/state/" {
		return http.HandlerFunc(handleConfigCheck)
	}

	// Any other function should require an authed session
	return sessionHandler
}

type AuthenticateRequestBody struct {
	Password string `json:"password"`
}

func (t api) authenticate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody AuthenticateRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	dkmToken, dkmError, err := t.dkm.Authenticate(requestBody.Password)
	if err != nil {
		sendErrorResponse(w, 500, err.Error())
		return
	}

	if dkmError != nil {
		sendErrorResponse(w, 403, dkmError.Error())
		return
	}

	if dkmToken == "" {
		// Wrong password.
		sendErrorResponse(w, 403, "Invalid password")
		return
	}

	// We've authed. Save our dkm authentication token to a new session.
	token, session := newSession()
	session.DKM_TOKEN = dkmToken
	storeSession(session, t.config)

	sendResponse(w, map[string]any{
		"success": true,
		"token":   token,
	})
}

func (t api) logout(w http.ResponseWriter, r *http.Request) {
	session, sessionOK := getSession(r, getBearerToken)
	if !sessionOK {
		sendErrorResponse(w, 500, "Failed to fetch session")
		return
	}

	// Clear our DKM token first. This ensures we can still convey an error
	// to the user if this fails for whatever reason. UI should tell them to
	// reboot their box or something to clear all authed sessions.
	ok, err := t.dkm.InvalidateToken(session.DKM_TOKEN)
	if err != nil {
		log.Println("failed to invalidate token with DKM:", err)
		sendErrorResponse(w, 500, err.Error())
		return
	}

	if !ok {
		log.Println("DKM returned ok=false when invalidating token")
		sendErrorResponse(w, 500, "Failed to invalidate token")
		return
	}

	delSession(r)

	sendResponse(w, map[string]any{
		"success": true,
	})
}
