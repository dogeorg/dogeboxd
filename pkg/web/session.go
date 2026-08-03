package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	authenticatev1 "github.com/Dogebox-WG/dogeboxd/protocol/gen/authenticate/v1"
)

var sessions *sessionManager

type sessionContextKey struct{}

func getBearerToken(r *http.Request) (bool, string) {
	authHeader := r.Header.Get("authorization")

	if authHeader == "" {
		return false, ""
	}

	authPart := strings.Fields(authHeader)

	if len(authPart) != 2 || !strings.EqualFold(authPart[0], "Bearer") {
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
	if !tokenOK || token == "" || sessions == nil {
		return Session{}, false
	}

	session, ok := sessions.Get(token)
	if !ok {
		return Session{}, false
	}
	return *session, true
}

func delSession(r *http.Request) error {
	tokenOK, token := getBearerToken(r)
	if !tokenOK || token == "" || sessions == nil {
		return errors.New("failed to fetch bearer token")
	}
	sessions.Revoke(token)
	return nil
}

func sessionFromContext(ctx context.Context) (*Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(*Session)
	return session, ok
}

func authReq(dbx dogeboxd.Dogeboxd, sm dogeboxd.StateManager, route string, auth_state AuthState, next http.HandlerFunc) http.HandlerFunc {
	tokenExtractor := getBearerToken
	if strings.HasPrefix(route, "/ws/") {
		tokenExtractor = getQueryToken
	}

	sessionHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenOK, token := tokenExtractor(r)
		if !tokenOK || sessions == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		session, ok := sessions.Get(token)

		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey{}, session)
		next.ServeHTTP(w, r.WithContext(ctx))
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

	return sessionHandler
}

type AuthenticateServer struct {
	a api
}

func (s *AuthenticateServer) Authenticate(
	_ context.Context,
	req *authenticatev1.AuthenticateRequest,
) (*authenticatev1.AuthenticateResponse, error) {
	authentication, dkmError, err := s.a.dkm.Authenticate(req.Password)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if dkmError != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, dkmError)
	}
	if authentication.AuthenticationToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("Invalid password"))
	}

	session := sessions.Create(authentication)

	res := &authenticatev1.AuthenticateResponse{
		Token:                session.Token,
		ExpiresAtUnixSeconds: uint32(session.Expiration.Unix()),
	}
	return res, nil
}

func (t api) logout(w http.ResponseWriter, r *http.Request) {
	tokenOK, token := getBearerToken(r)
	if !tokenOK || sessions == nil {
		sendErrorResponse(w, http.StatusUnauthorized, "Failed to fetch session")
		return
	}
	sessions.Revoke(token)

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

	// Invalidate all existing sessions since they're using the old password.
	if sessions != nil {
		sessions.RevokeAll()
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}
