package core

import (
	"encoding/json"
	"os"
	"path/filepath"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

const migrationsStateFilename = "migrations.json"

type MigrationRecord struct {
	RanSuccessfully bool           `json:"ranSuccessfully"`
	DoNotRun        bool           `json:"doNotRun"`
	Config          map[string]any `json:"config,omitempty"`
}

type State map[string]MigrationRecord

func getStatePath(config dogeboxd.ServerConfig) string {
	return filepath.Join(config.DataDir, migrationsStateFilename)
}

func LoadState(config dogeboxd.ServerConfig) (State, error) {
	statePath := getStatePath(config)
	contents, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return nil, err
	}

	if len(contents) == 0 {
		return State{}, nil
	}

	var state State
	if err := json.Unmarshal(contents, &state); err != nil {
		return nil, err
	}
	if state == nil {
		return State{}, nil
	}

	return state, nil
}

func SaveState(config dogeboxd.ServerConfig, state State) error {
	if state == nil {
		state = State{}
	}

	statePath := getStatePath(config)
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return err
	}

	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(statePath), "migrations-*.json")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(contents); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, statePath)
}

func SetDoNotRun(config dogeboxd.ServerConfig, migrationName string, value bool) error {
	state, err := LoadState(config)
	if err != nil {
		return err
	}

	record := state[migrationName]
	record.DoNotRun = value
	state[migrationName] = record

	return SaveState(config, state)
}

func SetRanSuccessfully(config dogeboxd.ServerConfig, migrationName string, value bool) error {
	state, err := LoadState(config)
	if err != nil {
		return err
	}

	record := state[migrationName]
	record.RanSuccessfully = value
	state[migrationName] = record

	return SaveState(config, state)
}

func MarkCompleted(config dogeboxd.ServerConfig, migrationName string) error {
	state, err := LoadState(config)
	if err != nil {
		return err
	}

	record := state[migrationName]
	record.RanSuccessfully = true
	record.DoNotRun = true
	state[migrationName] = record

	return SaveState(config, state)
}

func (r MigrationRecord) BoolConfig(name string) bool {
	if r.Config == nil {
		return false
	}

	value, ok := r.Config[name]
	if !ok {
		return false
	}

	flag, ok := value.(bool)
	return ok && flag
}
