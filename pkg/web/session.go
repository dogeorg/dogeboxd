package web

import (
	"context"
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

	"connectrpc.com/connect"
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	authenticatev1 "github.com/Dogebox-WG/dogeboxd/protocol/gen/authenticate/v1"
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

func authReq(dbx dogeboxd.Dogeboxd, sm dogeboxd.StateManager, route string, auth_state AuthState, next http.HandlerFunc) http.HandlerFunc {
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

	// Handle routes that are open as long as the dogebox is not fully configured
	if auth_state == ConfiguredAuth {
		return http.HandlerFunc(handleConfigCheck)
	}

	// Handle Websocket request authentication separately.
	if strings.HasPrefix(route, "/ws/") {
		tokenExtractor = getQueryToken
	}

	return sessionHandler
}

type AuthenticateServer struct {
	a api
}

func (s *AuthenticateServer) Authenticate(
	_ context.Context,
	req *authenticatev1.AuthenticateRequest,
) (*authenticatev1.AuthenticateResponse, error) {
	dkmToken, dkmError, err := s.a.dkm.Authenticate(req.Password)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if dkmError != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, dkmError)
	}
	if dkmToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("Invalid password"))
	}

	token, session := newSession()
	session.DKM_TOKEN = dkmToken
	storeSession(session, s.a.config)

	res := &authenticatev1.AuthenticateResponse{Token: token}
	return res, nil
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

type ChangePasswordRequestBody struct {
	CurrentPassword string `json:"current_password"`
	Seedphrase      string `json:"seedphrase"`
	NewPassword     string `json:"new_password"`
}

func (t api) changePassword(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody ChangePasswordRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	// Validate that either current_password or seedphrase is provided
	if requestBody.CurrentPassword == "" && requestBody.Seedphrase == "" {
		sendErrorResponse(w, 400, "Either current_password or seedphrase must be provided")
		return
	}

	// Change the password
	if err := t.dkm.ChangePassword(requestBody.CurrentPassword, requestBody.Seedphrase, requestBody.NewPassword); err != nil {
		// Map DKM error codes to appropriate HTTP responses

		log.Println("DKM error:", err)
		switch err.Error() {
		case "password":
			sendErrorResponse(w, 403, "Invalid credentials")
		case "newpassword":
			sendErrorResponse(w, 400, "New password cannot be empty")
		case "bad-request":
			sendErrorResponse(w, 400, "Invalid request format")
		case "nokey":
			sendErrorResponse(w, 400, "Master key not created")
		case "auth":
			sendErrorResponse(w, 400, "Authentication method required")
		case "length":
			sendErrorResponse(w, 403, "Invalid mnemonic length")
		case "seedphrase":
			sendErrorResponse(w, 403, "Invalid mnemonic word")
		default:
			sendErrorResponse(w, 500, "Failed to change password: "+err.Error())
		}
		return
	}

	// Invalidate all existing sessions since they're using the old password
	for _, session := range sessions {
		t.dkm.InvalidateToken(session.DKM_TOKEN)
	}
	sessions = nil

	sendResponse(w, map[string]any{
		"success": true,
	})
}
