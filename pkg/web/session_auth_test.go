package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

type sessionTestStateManager struct {
	state dogeboxd.State
}

func (m *sessionTestStateManager) Get() dogeboxd.State {
	return m.state
}

func (m *sessionTestStateManager) CloseDB() error {
	return nil
}

func (m *sessionTestStateManager) OpenDB() error {
	return nil
}

func (m *sessionTestStateManager) SetNetwork(state dogeboxd.NetworkState) error {
	m.state.Network = state
	return nil
}

func (m *sessionTestStateManager) SetDogebox(state dogeboxd.DogeboxState) error {
	m.state.Dogebox = state
	return nil
}

func (m *sessionTestStateManager) SetSources(state dogeboxd.SourceState) error {
	m.state.Sources = state
	return nil
}

func TestConfiguredStateSocketAllowsAnonymousSetupOnly(t *testing.T) {
	stateManager := &sessionTestStateManager{}
	called := false
	handler := authReq(
		dogeboxd.Dogeboxd{},
		stateManager,
		"/ws/state/",
		ConfiguredAuth,
		func(http.ResponseWriter, *http.Request) { called = true },
	)

	request := httptest.NewRequest(http.MethodGet, "/ws/state/", nil)
	response := httptest.NewRecorder()
	handler(response, request)

	if !called {
		t.Fatal("anonymous setup state socket was rejected")
	}

	called = false
	stateManager.state.Dogebox.InitialState.HasFullyConfigured = true
	response = httptest.NewRecorder()
	handler(response, request)
	if called {
		t.Fatal("configured state socket admitted an anonymous request")
	}
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("configured anonymous response = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestConfiguredStateSocketAcceptsActiveSession(t *testing.T) {
	stateManager := &sessionTestStateManager{}
	stateManager.state.Dogebox.InitialState.HasFullyConfigured = true
	sessions = newSessionManager(dogeboxd.ServerConfig{}, &sessionTestDKM{})
	session := sessions.Create(dogeboxd.DKMResponseAuthenticate{
		AuthenticationToken: "dkm-token",
		ValidFor:            60,
	})
	t.Cleanup(func() { sessions.Revoke(session.Token) })

	called := false
	handler := authReq(
		dogeboxd.Dogeboxd{},
		stateManager,
		"/ws/state/",
		ConfiguredAuth,
		func(http.ResponseWriter, *http.Request) { called = true },
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/ws/state/?token="+session.Token,
		nil,
	)
	response := httptest.NewRecorder()
	handler(response, request)

	if !called {
		t.Fatal("configured state socket rejected an active session")
	}
}
