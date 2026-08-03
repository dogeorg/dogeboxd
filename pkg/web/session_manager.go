package web

import (
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/gorilla/securecookie"
)

const (
	sessionExpiry        = time.Hour
	maxInvalidationRetry = 30 * time.Second
)

type Session struct {
	Token         string
	Expiration    time.Time
	DKM_TOKEN     string
	DKMExpiration time.Time

	revoked chan struct{}
	timer   *time.Timer
}

func (s *Session) Done() <-chan struct{} {
	return s.revoked
}

type sessionManager struct {
	mu        sync.RWMutex
	persistMu sync.Mutex
	sessions  map[string]*Session
	config    dogeboxd.ServerConfig
	dkm       dogeboxd.DKMManager
	now       func() time.Time
	retry     time.Duration
}

func newSessionManager(config dogeboxd.ServerConfig, dkm dogeboxd.DKMManager) *sessionManager {
	manager := &sessionManager{
		sessions: make(map[string]*Session),
		config:   config,
		dkm:      dkm,
		now:      time.Now,
		retry:    time.Second,
	}
	manager.load()
	return manager
}

func (m *sessionManager) Create(authentication dogeboxd.DKMResponseAuthenticate) *Session {
	now := m.now()
	expiration := now.Add(sessionExpiry)
	dkmExpiration := now
	if authentication.ValidFor > 0 {
		dkmExpiration = now.Add(time.Duration(authentication.ValidFor) * time.Second)
	}
	if dkmExpiration.Before(expiration) {
		expiration = dkmExpiration
	}

	tokenBytes := securecookie.GenerateRandomKey(32)
	tokenHex := make([]byte, hex.EncodedLen(len(tokenBytes)))
	hex.Encode(tokenHex, tokenBytes)
	session := &Session{
		Token:         string(tokenHex),
		Expiration:    expiration,
		DKM_TOKEN:     authentication.AuthenticationToken,
		DKMExpiration: dkmExpiration,
		revoked:       make(chan struct{}),
	}
	m.store(session)
	return session
}

func (m *sessionManager) Get(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}

	m.mu.RLock()
	session, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !m.now().Before(session.Expiration) {
		m.Revoke(token)
		return nil, false
	}
	return session, true
}

func (m *sessionManager) Revoke(token string) bool {
	m.mu.Lock()
	session, ok := m.sessions[token]
	if !ok {
		m.mu.Unlock()
		return false
	}
	delete(m.sessions, token)
	if session.timer != nil {
		session.timer.Stop()
	}
	close(session.revoked)
	m.mu.Unlock()

	m.persist()
	go m.invalidateDKMToken(session)
	return true
}

func (m *sessionManager) RevokeAll() {
	m.mu.RLock()
	tokens := make([]string, 0, len(m.sessions))
	for token := range m.sessions {
		tokens = append(tokens, token)
	}
	m.mu.RUnlock()

	for _, token := range tokens {
		m.Revoke(token)
	}
}

func (m *sessionManager) store(session *Session) {
	m.mu.Lock()
	m.sessions[session.Token] = session
	m.scheduleExpiryLocked(session)
	m.mu.Unlock()
	m.persist()
}

func (m *sessionManager) restore(session Session) bool {
	if !m.now().Before(session.Expiration) {
		go m.invalidateDKMToken(&session)
		return false
	}
	if session.DKMExpiration.IsZero() {
		// Older development session files did not record the DKM deadline.
		session.DKMExpiration = session.Expiration
	}
	session.revoked = make(chan struct{})
	m.mu.Lock()
	m.sessions[session.Token] = &session
	m.scheduleExpiryLocked(&session)
	m.mu.Unlock()
	return true
}

func (m *sessionManager) scheduleExpiryLocked(session *Session) {
	delay := session.Expiration.Sub(m.now())
	if delay < 0 {
		delay = 0
	}
	session.timer = time.AfterFunc(delay, func() {
		m.Revoke(session.Token)
	})
}

func (m *sessionManager) invalidateDKMToken(session *Session) {
	if m.dkm == nil || session.DKM_TOKEN == "" {
		return
	}

	retryDelay := m.retry
	for {
		ok, err := m.dkm.InvalidateToken(session.DKM_TOKEN)
		if err == nil && ok {
			return
		}

		remaining := session.DKMExpiration.Sub(m.now())
		if remaining <= 0 {
			log.Printf("Failed to invalidate expired DKM session token: %v", err)
			return
		}
		if retryDelay > remaining {
			retryDelay = remaining
		}
		time.Sleep(retryDelay)
		if retryDelay < maxInvalidationRetry {
			retryDelay *= 2
			if retryDelay > maxInvalidationRetry {
				retryDelay = maxInvalidationRetry
			}
		}
	}
}

func (m *sessionManager) load() {
	if !m.config.DevMode {
		return
	}

	path := m.persistencePath()
	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to open dev-sessions.gob: %v", err)
		}
		return
	}
	defer file.Close()

	var loaded []Session
	if err := gob.NewDecoder(file).Decode(&loaded); err != nil {
		log.Printf("Failed to decode sessions from dev-sessions.gob: %v", err)
		return
	}

	restored := 0
	for _, session := range loaded {
		if m.restore(session) {
			restored++
		}
	}
	log.Printf("Loaded %d valid development sessions", restored)
	m.persist()
}

func (m *sessionManager) persist() {
	if !m.config.DevMode {
		return
	}
	m.persistMu.Lock()
	defer m.persistMu.Unlock()

	m.mu.RLock()
	snapshot := make([]Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		snapshot = append(snapshot, Session{
			Token:         session.Token,
			Expiration:    session.Expiration,
			DKM_TOKEN:     session.DKM_TOKEN,
			DKMExpiration: session.DKMExpiration,
		})
	}
	m.mu.RUnlock()

	file, err := os.OpenFile(m.persistencePath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		log.Printf("Failed to open dev-sessions.gob: %v", err)
		return
	}
	defer file.Close()
	if err := gob.NewEncoder(file).Encode(snapshot); err != nil {
		log.Printf("Failed to encode sessions to dev-sessions.gob: %v", err)
	}
}

func (m *sessionManager) persistencePath() string {
	return fmt.Sprintf("%s/dev-sessions.gob", m.config.DataDir)
}
