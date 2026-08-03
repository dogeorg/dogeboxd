package web

import (
	"encoding/gob"
	"errors"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

type sessionTestDKM struct {
	mu                sync.Mutex
	invalidationCalls int
	failuresRemaining int
	invalidated       chan string
}

func (d *sessionTestDKM) CreateKey(string) ([]string, error) {
	return nil, nil
}

func (d *sessionTestDKM) Authenticate(string) (dogeboxd.DKMResponseAuthenticate, error, error) {
	return dogeboxd.DKMResponseAuthenticate{}, nil, nil
}

func (d *sessionTestDKM) RefreshToken(string) (string, bool, error) {
	return "", false, nil
}

func (d *sessionTestDKM) InvalidateToken(token string) (bool, error) {
	d.mu.Lock()
	d.invalidationCalls++
	if d.failuresRemaining > 0 {
		d.failuresRemaining--
		d.mu.Unlock()
		return false, errors.New("temporary DKM failure")
	}
	d.mu.Unlock()
	select {
	case d.invalidated <- token:
	default:
	}
	return true, nil
}

func (d *sessionTestDKM) MakeDelegate(string, string) (dogeboxd.DKMResponseMakeDelegate, error) {
	return dogeboxd.DKMResponseMakeDelegate{}, nil
}

func (d *sessionTestDKM) ChangePassword(string, string, string) error {
	return nil
}

func TestSessionManagerUsesShortestCredentialLifetime(t *testing.T) {
	manager := newSessionManager(dogeboxd.ServerConfig{}, &sessionTestDKM{})
	now := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return now }

	session := manager.Create(dogeboxd.DKMResponseAuthenticate{
		AuthenticationToken: "dkm-token",
		ValidFor:            30,
	})

	if got, want := session.Expiration, now.Add(30*time.Second); !got.Equal(want) {
		t.Fatalf("session expiry = %v, want %v", got, want)
	}
	manager.Revoke(session.Token)
}

func TestSessionManagerDoesNotExtendMissingCredentialLifetime(t *testing.T) {
	manager := newSessionManager(dogeboxd.ServerConfig{}, &sessionTestDKM{})
	now := time.Unix(1_700_000_000, 0)
	manager.now = func() time.Time { return now }

	session := manager.Create(dogeboxd.DKMResponseAuthenticate{
		AuthenticationToken: "dkm-token",
	})

	if !session.Expiration.Equal(now) {
		t.Fatalf("session expiry = %v, want %v", session.Expiration, now)
	}
}

func TestSessionExpiryRevokesOnceAndRetriesDKMInvalidation(t *testing.T) {
	dkm := &sessionTestDKM{
		failuresRemaining: 2,
		invalidated:       make(chan string, 1),
	}
	manager := newSessionManager(dogeboxd.ServerConfig{}, dkm)
	manager.retry = time.Millisecond
	session := &Session{
		Token:         "web-token",
		Expiration:    time.Now().Add(10 * time.Millisecond),
		DKM_TOKEN:     "dkm-token",
		DKMExpiration: time.Now().Add(time.Second),
		revoked:       make(chan struct{}),
	}
	manager.store(session)

	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("session was not revoked at expiry")
	}
	select {
	case token := <-dkm.invalidated:
		if token != "dkm-token" {
			t.Fatalf("invalidated token = %q, want dkm-token", token)
		}
	case <-time.After(time.Second):
		t.Fatal("DKM token was not invalidated")
	}

	manager.Revoke(session.Token)
	dkm.mu.Lock()
	calls := dkm.invalidationCalls
	dkm.mu.Unlock()
	if calls != 3 {
		t.Fatalf("DKM invalidation calls = %d, want 3", calls)
	}
}

func TestSessionManagerConcurrentLookupAndRevocation(t *testing.T) {
	manager := newSessionManager(dogeboxd.ServerConfig{}, &sessionTestDKM{})
	session := manager.Create(dogeboxd.DKMResponseAuthenticate{
		AuthenticationToken: "dkm-token",
		ValidFor:            60,
	})

	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func(operation int) {
			defer workers.Done()
			if operation%2 == 0 {
				manager.Get(session.Token)
				return
			}
			manager.Revoke(session.Token)
		}(i)
	}
	workers.Wait()

	if _, ok := manager.Get(session.Token); ok {
		t.Fatal("revoked session remained available")
	}
}

func TestSessionManagerRestoresOnlyValidDevelopmentSessions(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now()
	file, err := os.Create(dataDir + "/dev-sessions.gob")
	if err != nil {
		t.Fatal(err)
	}
	loaded := []Session{
		{
			Token:         "valid-web-token",
			Expiration:    now.Add(time.Minute),
			DKM_TOKEN:     "valid-dkm-token",
			DKMExpiration: now.Add(time.Minute),
		},
		{
			Token:         "expired-web-token",
			Expiration:    now.Add(-time.Minute),
			DKM_TOKEN:     "expired-dkm-token",
			DKMExpiration: now.Add(-time.Minute),
		},
	}
	if err := gob.NewEncoder(file).Encode(loaded); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	dkm := &sessionTestDKM{invalidated: make(chan string, 2)}
	manager := newSessionManager(dogeboxd.ServerConfig{
		DevMode: true,
		DataDir: dataDir,
	}, dkm)
	t.Cleanup(func() { manager.Revoke("valid-web-token") })

	if _, ok := manager.Get("valid-web-token"); !ok {
		t.Fatal("valid development session was not restored")
	}
	if _, ok := manager.Get("expired-web-token"); ok {
		t.Fatal("expired development session was restored")
	}
}

func TestGetBearerTokenRequiresBearerScheme(t *testing.T) {
	tests := []struct {
		header string
		ok     bool
	}{
		{header: "Bearer token", ok: true},
		{header: "bearer token", ok: true},
		{header: "Basic token", ok: false},
		{header: "Bearer", ok: false},
		{header: "Bearer token extra", ok: false},
	}

	for _, test := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", test.header)
		ok, _ := getBearerToken(req)
		if ok != test.ok {
			t.Errorf("getBearerToken(%q) ok = %t, want %t", test.header, ok, test.ok)
		}
	}
}
