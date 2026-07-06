package system

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

var (
	_       dogeboxd.StateManager = &StateManager{} // interface guard
	current string                = "0"             // Key for singletons in the database
)

func NewStateManager(store *dogeboxd.StoreManager) dogeboxd.StateManager {
	setupSessionID := generateSetupSessionID()

	// Set initial state
	s := &StateManager{
		storeManager: store,
		netStore:     dogeboxd.GetTypeStore[dogeboxd.NetworkState](store),
		dbxStore:     dogeboxd.GetTypeStore[dogeboxd.DogeboxState](store),
		srcStore:     dogeboxd.GetTypeStore[dogeboxd.SourceState](store),
		network: dogeboxd.NetworkState{
			CurrentNetwork: nil,
			PendingNetwork: nil,
		},
		dogebox: dogeboxd.DogeboxState{
			InitialState: dogeboxd.DogeboxStateInitialSetup{
				SetupSessionID:     setupSessionID,
				HasGeneratedKey:    false,
				HasSetNetwork:      false,
				HasFullyConfigured: false,
			},
			Flags: dogeboxd.DogeboxFlags{
				IsFirstTimeWelcomeComplete: false,
				IsDeveloperMode:            false,
			},
		},
		source: dogeboxd.SourceState{
			SourceConfigs: []dogeboxd.ManifestSourceConfiguration{},
		},
	}

	// try loading state from the DB
	net, err := s.netStore.Get(current)
	if err != nil {
		fmt.Println(">> couldn't load network state, using default")
	} else {
		s.network = net
	}

	dbx, err := s.dbxStore.Get(current)
	if err != nil {
		fmt.Println(">> couldn't load dbx state, using default")
	} else {
		s.dogebox = dbx
	}

	if s.dogebox.InitialState.SetupSessionID == "" {
		s.dogebox.InitialState.SetupSessionID = generateSetupSessionID()
		if err := s.dbxStore.Set(current, s.dogebox); err != nil {
			log.Printf(">> couldn't persist setup session id, using generated value in memory: %v", err)
		}
	}

	src, err := s.srcStore.Get(current)
	if err != nil {
		fmt.Println(">> couldn't load src state, using default")
	} else {
		s.source = src
	}

	return s
}

func generateSetupSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return current
	}

	return hex.EncodeToString(buf)
}

type StateManager struct {
	storeManager *dogeboxd.StoreManager
	netStore     *dogeboxd.TypeStore[dogeboxd.NetworkState]
	dbxStore     *dogeboxd.TypeStore[dogeboxd.DogeboxState]
	srcStore     *dogeboxd.TypeStore[dogeboxd.SourceState]
	network      dogeboxd.NetworkState
	dogebox      dogeboxd.DogeboxState
	source       dogeboxd.SourceState
}

func (s *StateManager) Get() dogeboxd.State {
	return dogeboxd.State{
		Network: s.network,
		Dogebox: s.dogebox,
		Sources: s.source,
	}
}

func (s *StateManager) CloseDB() error {
	return s.storeManager.CloseDB()
}

func (s *StateManager) OpenDB() error {
	return s.storeManager.OpenDB()
}

func (s *StateManager) SetNetwork(ns dogeboxd.NetworkState) error {
	s.network = ns
	return s.netStore.Set(current, s.network)
}

func (s *StateManager) SetDogebox(dbs dogeboxd.DogeboxState) error {
	s.dogebox = dbs
	return s.dbxStore.Set(current, s.dogebox)
}

func (s *StateManager) SetSources(state dogeboxd.SourceState) error {
	s.source = state
	return s.srcStore.Set(current, s.source)
}
