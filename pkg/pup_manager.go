package dogeboxd

import (
	"crypto/rand"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

/* The PupManager is collection of PupState and PupStats
* for all installed Pups.
*
* It supports subscribing to changes and ensures pups
* are persisted to disk.
 */

type PupManager struct {
	pupDir string // Where pup state is stored
	state  map[string]*PupState
	stats  map[string]*PupStats
}

func NewPupManager(pupDir string) (PupManager, error) {
	p := PupManager{
		pupDir: pupDir,
		state:  map[string]*PupState{},
		stats:  map[string]*PupStats{},
	}
	// load pups from disk
	return p, p.loadPups()
}

/* This method is used to add a new pup from a manifest
* and init it's values to then be configured by the user
* and dogebox system. See PurgePup() for the opposite.
*
* Once a pup has been initialised it is considered 'managed'
* by the PupManager until purged.
 */
func (t PupManager) AdoptPup(m PupManifest) error {
	// Create a PupID for this new Pup
	var PupID string
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return err
	}
	PupID = fmt.Sprintf("%x", b)

	// Set up initial PupState and save it to disk
	p := PupState{
		ID:           PupID,
		Manifest:     m,
		Config:       map[string]string{},
		Installation: STATE_INSTALLING,
		Enabled:      false,
		NeedsConf:    false, // TODO
		NeedsDeps:    false, // TODO
		Version:      "TODO",
	}
	err = t.savePup(&p)
	if err != nil {
		return err
	}

	// If we've successfully saved to disk, set up in-memory.
	t.indexPup(&p)
	return nil
}

func (t PupManager) GetStateMap() map[string]PupState {
	out := map[string]PupState{}
	for k, v := range t.state {
		out[k] = *v
	}
	return out
}

func (t PupManager) GetStatsMap() map[string]PupStats {
	out := map[string]PupStats{}
	for k, v := range t.stats {
		out[k] = *v
	}
	return out
}

func (t PupManager) GetPup(id string) (PupState, PupStats, error) {
	state, ok := t.state[id]
	if ok {
		return *state, *t.stats[id], nil
	}
	return PupState{}, PupStats{}, errors.New("pup not found")
}

/* Updating a PupState follows the veradic update func pattern
* to accept multiple types of updates before saving to disk as
* an atomic update.
*
* ie: err := manager.UpdatePup(id, SetPupInstallation(STATE_READY))
* see bottom of file for options
 */
func (t PupManager) UpdatePup(id string, updates ...func(*PupState)) error {
	p, ok := t.state[id]
	if !ok {
		return errors.New("pup not found")
	}
	for _, updateFn := range updates {
		updateFn(p)
	}

	return t.savePup(p)
}

/* Gets the list of previously managed pupIDs and loads their
* state into memory.
 */
func (t PupManager) loadPups() error {
	// find pup save files
	pupSaveFiles := []string{}
	files, err := os.ReadDir(t.pupDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".gob") {
			pupSaveFiles = append(pupSaveFiles, filepath.Join(t.pupDir, file.Name()))
		}
	}

	for _, path := range pupSaveFiles {
		file, err := os.Open(path)
		if err != nil {
			fmt.Sprintf("Cannot find state for pup %s at %q: %w\n", path, err)
			continue
		}
		defer file.Close()

		state := PupState{}
		decoder := gob.NewDecoder(file)
		if err := decoder.Decode(&state); err != nil {
			if err == io.EOF {
				fmt.Sprintf("file %q is empty, skipping %s", path)
			}
			fmt.Sprintf("cannot decode object from file %q: %w", path, err)
			continue
		}

		// Success! add to index
		t.indexPup(&state)
	}
	return nil
}

func (t PupManager) savePup(p *PupState) error {
	path := filepath.Join(t.pupDir, fmt.Sprintf("pup_%s.gob", p.ID))
	tempFile, err := os.CreateTemp("", fmt.Sprintf("temp_%s", p.ID))
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(p); err != nil {
		return fmt.Errorf("cannot encode object: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), path); err != nil {
		return fmt.Errorf("cannot rename temporary file to %q: %w", path, err)
	}

	return nil
}

func (t PupManager) indexPup(p *PupState) {
	s := PupStats{
		ID:       p.ID,
		Status:   STATE_STOPPED,
		StatCPU:  NewFloatBuffer(32),
		StatMEM:  NewFloatBuffer(32),
		StatDISK: NewFloatBuffer(32),
	}

	t.state[p.ID] = p
	t.stats[p.ID] = &s
}

/*****************************************************************************/
/*                Varadic Update Funcs for PupManager.UpdatePup:             */
/*****************************************************************************/

func SetPupInstallation(state string) func(*PupState) {
	return func(p *PupState) {
		p.Installation = state
	}
}

func SetPupConfig(newFields map[string]string) func(*PupState) {
	return func(p *PupState) {
		for k, v := range newFields {
			p.Config[k] = v
		}
	}
}

func PupEnabled(b bool) func(*PupState) {
	return func(p *PupState) {
		p.Enabled = b
	}
}

func PupNeedsConf(b bool) func(*PupState) {
	return func(p *PupState) {
		p.NeedsConf = b
	}
}

func PupNeedsDeps(b bool) func(*PupState) {
	return func(p *PupState) {
		p.NeedsDeps = b
	}
}
