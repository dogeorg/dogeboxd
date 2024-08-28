package system

import (
	"bytes"
	"encoding/gob"
	"log"
	"os"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.StateManager = &StateManager{}

func NewStateManager() dogeboxd.StateManager {
	gob.Register(dogeboxd.SelectedNetworkEthernet{})
	gob.Register(dogeboxd.SelectedNetworkWifi{})
	gob.Register(dogeboxd.DogeboxStateInitialSetup{})
	return &StateManager{}
}

type StateManager struct {
	network    dogeboxd.NetworkState
	dogebox    dogeboxd.DogeboxState
	repository dogeboxd.RepositoryState
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
	m.repository = dogeboxd.RepositoryState{
		Repositories: []dogeboxd.ManifestRepository{},
	}
}

func (m StateManager) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)

	if err := encoder.Encode(m.network); err != nil {
		return nil, err
	}

	if err := encoder.Encode(m.dogebox); err != nil {
		return nil, err
	}

	if err := encoder.Encode(m.repository); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (m *StateManager) GobDecode(data []byte) error {
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)

	if err := decoder.Decode(&m.network); err != nil {
		return err
	}

	if err := decoder.Decode(&m.dogebox); err != nil {
		return err
	}

	if err := decoder.Decode(&m.repository); err != nil {
		return err
	}

	return nil
}

func (s *StateManager) Get() dogeboxd.State {
	return dogeboxd.State{
		Network: s.network,
		Dogebox: s.dogebox,
	}
}

func (s *StateManager) SetNetwork(ns dogeboxd.NetworkState) {
	s.network = ns
}

func (s *StateManager) SetDogebox(dbs dogeboxd.DogeboxState) {
	s.dogebox = dbs
}

func (s *StateManager) SetRepository(r dogeboxd.RepositoryState) {
	s.repository = r
}

func (s *StateManager) Save() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	filePath := filepath.Join(homeDir, "dogeboxd.gob")
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	filePath := filepath.Join(homeDir, "dogeboxd.gob")
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No existing state file found. Starting with empty state.")
			s.reset()
			return nil
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
