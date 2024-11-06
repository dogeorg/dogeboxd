package system

import (
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var (
	_       dogeboxd.StateManager = &StateManager{} // interface guard
	current string                = "0"             // Key for singletons in the database
)

func NewStateManager(store *dogeboxd.StoreManager) dogeboxd.StateManager {
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
				HasGeneratedKey:    false,
				HasSetNetwork:      false,
				HasFullyConfigured: false,
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

	src, err := s.srcStore.Get(current)
	if err != nil {
		fmt.Println(">> couldn't load src state, using default")
	} else {
		s.source = src
	}

	return s
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
