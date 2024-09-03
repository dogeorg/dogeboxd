package dogeboxd

import (
	"crypto/rand"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/dogeorg/dogeboxd/pkg/pup"
)

/* The PupManager is collection of PupState and PupStats
* for all installed Pups.
*
* It supports subscribing to changes and ensures pups
* are persisted to disk.
 */

type PupManager struct {
	pupDir string // Where pup state is stored
	tmpDir string // Where temporary files are stored
	lastIP net.IP // last issued IP address
	state  map[string]*PupState
	stats  map[string]*PupStats
}

func NewPupManager(dataDir string) (PupManager, error) {
	pupDir := filepath.Join(dataDir, "pups")
	tmpDir := filepath.Join(dataDir, "tmp")

	if _, err := os.Stat(pupDir); os.IsNotExist(err) {
		log.Printf("Pup directory %q not found, creating it", pupDir)
		err = os.MkdirAll(pupDir, 0755)
		if err != nil {
			return PupManager{}, fmt.Errorf("failed to create pup directory: %w", err)
		}
	}

	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		log.Printf("Tmp directory %q not found, creating it", tmpDir)
		err = os.MkdirAll(tmpDir, 0755)
		if err != nil {
			return PupManager{}, fmt.Errorf("failed to create tmp directory: %w", err)
		}
	}

	p := PupManager{
		pupDir: pupDir,
		tmpDir: tmpDir,
		state:  map[string]*PupState{},
		stats:  map[string]*PupStats{},
	}
	// load pups from disk
	err := p.loadPups()
	if err != nil {
		return p, err
	}

	// set lastIP for IP Generation
	ip := net.IP{10, 0, 0, 1} // skip 0.1 (dogeboxd)
	for _, v := range p.state {
		ip2 := net.ParseIP(v.IP).To4()
		for i := 0; i < 4; i++ {
			if ip[i] < ip2[i] {
				ip = ip2
				break
			} else if ip[i] > ip2[i] {
				continue
			}
		}
	}
	p.lastIP = ip

	return p, nil
}

/* This method is used to add a new pup from a manifest
* and init it's values to then be configured by the user
* and dogebox system. See PurgePup() for the opposite.
*
* Once a pup has been initialised it is considered 'managed'
* by the PupManager until purged.
*
* Returns PupID, error
 */
func (t PupManager) AdoptPup(m pup.PupManifest, source ManifestSource) (string, error) {
	// Firstly (for now), check if we already have this manifest installed
	for _, p := range t.state {
		if m.Meta.Name == p.Manifest.Meta.Name && m.Meta.Version == p.Manifest.Meta.Version && p.Source.Name == source.Name() {
			return p.ID, errors.New("pup already installed")
		}
	}

	// Create a PupID for this new Pup
	var PupID string
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return PupID, err
	}
	PupID = fmt.Sprintf("%x", b)

	// Claim the next available IP
	for i := 3; i >= 0; i-- {
		t.lastIP[i]++
		if t.lastIP[i] > 0 {
			break
		}
		// If this octet wrapped, reset it to 0
		t.lastIP[i] = 0
	}

	// Check if we have gone off the edge of the world
	if t.lastIP[0] > 10 || (t.lastIP[0] == 10 && t.lastIP[1] > 0) {
		return PupID, errors.New("exhausted 65,536 IP addresses, what are you doing??")
	}

	// Set up initial PupState and save it to disk
	p := PupState{
		ID:           PupID,
		Source:       source.Config(),
		Manifest:     m,
		Config:       map[string]string{},
		Installation: STATE_INSTALLING,
		Enabled:      false,
		NeedsConf:    false, // TODO
		NeedsDeps:    false, // TODO
		IP:           t.lastIP.String(),
		Version:      m.Meta.Version,
	}
	err = t.savePup(&p)
	if err != nil {
		return PupID, err
	}

	// If we've successfully saved to disk, set up in-memory.
	t.indexPup(&p)
	return PupID, nil
}

func (t PupManager) PurgePup(pupId string) error {
	// Remove our in-memory state
	delete(t.state, pupId)
	delete(t.stats, pupId)

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

func (t PupManager) GetAllFromSource(source ManifestSourceConfiguration) []*PupState {
	pups := []*PupState{}

	for _, pup := range t.state {
		if pup.Source == source {
			pups = append(pups, pup)
		}
	}

	return pups
}

func (t PupManager) GetPupFromSource(name string, source ManifestSourceConfiguration) *PupState {
	for _, pup := range t.state {
		if pup.Source == source && pup.Manifest.Meta.Name == name {
			return pup
		}
	}
	return nil
}

/* Updating a PupState follows the veradic update func pattern
* to accept multiple types of updates before saving to disk as
* an atomic update.
*
* ie: err := manager.UpdatePup(id, SetPupInstallation(STATE_READY))
* see bottom of file for options
 */
func (t PupManager) UpdatePup(id string, updates ...func(*PupState)) (PupState, error) {
	p, ok := t.state[id]
	if !ok {
		return PupState{}, errors.New("pup not found")
	}
	for _, updateFn := range updates {
		updateFn(p)
	}

	return *p, t.savePup(p)
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
			fmt.Printf("Failed to open pup save file at %q: %v\n", path, err)
			continue
		}
		defer file.Close()

		state := PupState{}
		decoder := gob.NewDecoder(file)
		if err := decoder.Decode(&state); err != nil {
			if err == io.EOF {
				fmt.Printf("pup state at %q is empty, skipping\n", path)
			}
			fmt.Printf("cannot decode object from file %q: %v", path, err)
			continue
		}

		log.Printf("Loaded pup state: %+v", state)

		// Success! add to index
		t.indexPup(&state)
	}
	return nil
}

func (t PupManager) savePup(p *PupState) error {
	path := filepath.Join(t.pupDir, fmt.Sprintf("pup_%s.gob", p.ID))
	tempFile, err := os.CreateTemp(t.tmpDir, fmt.Sprintf("temp_%s", p.ID))
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
