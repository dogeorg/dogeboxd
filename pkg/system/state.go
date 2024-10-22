package system

import (
	"encoding/gob"
	"log"
	"os"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.StateManager = &StateManager{}

func NewStateManager(store *dogeboxd.StoreManager) dogeboxd.StateManager {
	return &StateManager{
		storeManager: storeManager,
	}
}

type StateManager struct {
	storeManager *dogeboxd.StoreManager
	network      dogeboxd.NetworkState
	dogebox      dogeboxd.DogeboxState
	source       dogeboxd.SourceState
}

func (m *StateManager) reset() {
	m.network = dogeboxd.NetworkState{
		CurrentNetwork: nil,
		PendingNetwork: nil,
	}
	m.dogebox = dogeboxd.DogeboxState{
		InitialState: dogeboxd.DogeboxStateInitialSetup{
			HasGeneratedKey:    false,
			HasSetNetwork:      false,
			HasFullyConfigured: false,
		},
	}
	m.source = dogeboxd.SourceState{
		SourceConfigs: []dogeboxd.ManifestSourceConfiguration{},
	}
}

func (s *StateManager) Get() dogeboxd.State {
	return dogeboxd.State{
		Network: s.network,
		Dogebox: s.dogebox,
		Sources: s.source,
	}
}

func (s *StateManager) SetNetwork(ns dogeboxd.NetworkState) {
	s.network = ns
}

func (s *StateManager) SetDogebox(dbs dogeboxd.DogeboxState) {
	s.dogebox = dbs
}

func (s *StateManager) SetSources(state dogeboxd.SourceState) {
	s.source = state
}

func (s *StateManager) Save() error {
	filePath := filepath.Join(s.dataDir, "dogeboxd.gob")
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(s)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateManager) Load() error {
	filePath := filepath.Join(s.dataDir, "dogeboxd.gob")
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No existing state file found. Starting with empty state.")
			s.reset()
			return s.Save()
		}
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	err = decoder.Decode(s)
	if err != nil {
		return err
	}

	log.Printf("State loaded from %s", filePath)
	log.Printf("Loaded state: %+v", s)
	return nil
}
