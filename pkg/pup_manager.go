package dogeboxd

import (
	"context"
	"crypto/rand"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/Masterminds/semver"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

const (
	PUP_CHANGED_INSTALLATION int = iota
	PUP_ADOPTED                  = iota
)

// Represents a change to pup state
type Pupdate struct {
	ID    string
	Event int // see consts above ^
	State PupState
}

/* The PupManager is collection of PupState and PupStats
* for all installed Pups.
*
* It supports subscribing to changes and ensures pups
* are persisted to disk.
 */

type PupManager struct {
	pupDir            string // Where pup state is stored
	tmpDir            string // Where temporary files are stored
	lastIP            net.IP // last issued IP address
	mu                *sync.Mutex
	state             map[string]*PupState
	stats             map[string]*PupStats
	updateSubscribers map[chan Pupdate]bool    // listeners for 'Pupdates'
	statsSubscribers  map[chan []PupStats]bool // listeners for 'PupStats'
	monitor           SystemMonitor
	sourceManager     SourceManager
}

func NewPupManager(dataDir string, tmpDir string, monitor SystemMonitor) (PupManager, error) {
	pupDir := filepath.Join(dataDir, "pups")

	if _, err := os.Stat(pupDir); os.IsNotExist(err) {
		log.Printf("Pup directory %q not found, creating it", pupDir)
		err = os.MkdirAll(pupDir, 0755)
		if err != nil {
			return PupManager{}, fmt.Errorf("failed to create pup directory: %w", err)
		}
	}

	mu := sync.Mutex{}
	p := PupManager{
		pupDir:            pupDir,
		tmpDir:            tmpDir,
		state:             map[string]*PupState{},
		stats:             map[string]*PupStats{},
		updateSubscribers: map[chan Pupdate]bool{},
		statsSubscribers:  map[chan []PupStats]bool{},
		mu:                &mu,
		monitor:           monitor,
	}
	// load pups from disk
	err := p.loadPups()
	if err != nil {
		return p, err
	}

	// set lastIP for IP Generation
	ip := net.IP{10, 69, 0, 1} // skip 0.1 (dogeboxd)
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
	p.updateMonitoredPups()
	return p, nil
}

func (t *PupManager) SetSourceManager(sourceManager SourceManager) {
	t.sourceManager = sourceManager
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
		if m.Meta.Name == p.Manifest.Meta.Name && m.Meta.Version == p.Manifest.Meta.Version && p.Source.ID == source.Config().ID {
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
	if t.lastIP[0] > 10 || (t.lastIP[0] == 10 && t.lastIP[1] > 70) {
		return PupID, errors.New("exhausted 65,534 IP addresses, what are you doing??")
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

	// update health details
	t.healthCheckPupState(&p)

	// Send a Pupdate announcing 'adopted'
	t.sendPupdate(Pupdate{
		ID:    PupID,
		Event: PUP_ADOPTED,
		State: p,
	})
	return PupID, nil
}

func (t PupManager) PurgePup(pupId string) error {
	// Remove our in-memory state
	delete(t.state, pupId)
	delete(t.stats, pupId)

	return nil
}

/* Hand out channels to pupdate subscribers */
func (t PupManager) GetUpdateChannel() chan Pupdate {
	ch := make(chan Pupdate)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.updateSubscribers[ch] = true
	return ch
}

// send pupdates to subscribers
func (t PupManager) sendPupdate(p Pupdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for ch := range t.updateSubscribers {
		select {
		case ch <- p:
			// sent pupdate to subscriber
		default:
			// channel is closed or full, delete it
			delete(t.updateSubscribers, ch)
		}
	}
}

/* Hand out channels to stat subscribers */
func (t PupManager) GetStatsChannel() chan []PupStats {
	ch := make(chan []PupStats)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.statsSubscribers[ch] = true
	return ch
}

// send stats to subscribers
func (t PupManager) sendStats() {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := []PupStats{}

	for _, v := range t.stats {
		stats = append(stats, *v)
	}

	for ch := range t.statsSubscribers {
		select {
		case ch <- stats:
			// sent stats to subscriber
		default:
			// channel is closed or full, delete it
			delete(t.statsSubscribers, ch)
		}
	}
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

func (t PupManager) FindPupByIP(ip string) (PupState, PupStats, error) {
	for _, p := range t.state {
		if ip == p.IP {
			return t.GetPup(p.ID)
		}
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
func (t PupManager) UpdatePup(id string, updates ...func(*PupState, *[]Pupdate)) (PupState, error) {
	p, ok := t.state[id]
	if !ok {
		return PupState{}, errors.New("pup not found")
	}

	// capture any pupdates from updateFns
	pupdates := []Pupdate{}
	for _, updateFn := range updates {
		updateFn(p, &pupdates)
	}

	// update pup healthcheck details before saving
	t.healthCheckPupState(p)

	// send any pupdates
	for _, pu := range pupdates {
		t.sendPupdate(pu)
	}

	return *p, t.savePup(p)
}

/* Run as a service so we can listen for stats from the
* SystemMonitor and update t.stats
 */
func (t PupManager) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
				select {
				case <-stop:
					break mainloop

				case stats := <-t.monitor.GetStatChannel():
					// turn ProcStatus into updates to t.state
					for k, v := range stats {
						id := k[strings.Index(k, "-")+1 : strings.Index(k, ".")]
						s, ok := t.stats[id]
						if !ok {
							fmt.Println("skipping stats for unfound pup", id)
							continue
						}
						s.StatCPU.Add(v.CPUPercent)
						s.StatMEM.Add(v.MEMMb)
						s.StatMEMPERC.Add(v.MEMPercent)
						s.StatDISK.Add(float64(0.0)) // TODO
						// Calculate our status
						p := t.state[id]
						if v.Running && p.Enabled {
							s.Status = STATE_RUNNING
						} else if v.Running && !p.Enabled {
							s.Status = STATE_STOPPING
						} else if !v.Running && p.Enabled {
							s.Status = STATE_STARTING
						} else {
							s.Status = STATE_STOPPED
						}
						t.healthCheckPupState(p)
					}
					t.sendStats()

				case stats := <-t.monitor.GetFastStatChannel():
					// This will recieve stats rapidly when pups
					// are changing state (shutting down, starting up)
					// these should not be recorded in the floatBuffers
					// but only to rapidly track STATUS change
					for k, v := range stats {
						id := k[strings.Index(k, "-")+1 : strings.Index(k, ".")]
						s, ok := t.stats[id]
						if !ok {
							fmt.Println("skipping stats for unfound pup", id)
							continue
						}
						// Calculate our status
						p := t.state[id]
						if v.Running && p.Enabled {
							s.Status = STATE_RUNNING
						} else if v.Running && !p.Enabled {
							s.Status = STATE_STOPPING
						} else if !v.Running && p.Enabled {
							s.Status = STATE_STARTING
						} else {
							s.Status = STATE_STOPPED
						}
						t.healthCheckPupState(p)
					}
					t.sendStats()
				}
			}
		}()
		started <- true
		<-stop
		// do shutdown things
		stopped <- true
	}()
	return nil
}

func (t PupManager) GetMetrics(pupId string) map[string]interface{} {
	s, ok := t.stats[pupId]
	if !ok {
		fmt.Printf("Error: Unable to find stats for pup %s\n", pupId)
		return map[string]interface{}{}
	}

	metrics := make(map[string]interface{})
	for name, buffer := range s.Metrics {
		switch b := buffer.(type) {
		case *Buffer[string]:
			metrics[name] = b.GetValues()
		case *Buffer[int]:
			metrics[name] = b.GetValues()
		case *Buffer[float64]:
			metrics[name] = b.GetValues()
		default:
			fmt.Printf("Warning: Unknown buffer type for metric %s\n", name)
		}
	}

	return metrics
}

// Updates the stats.Metrics field with data from the pup router
func (t PupManager) UpdateMetrics(u UpdateMetrics) {
	s, ok := t.stats[u.PupID]
	if !ok {
		fmt.Println("skipping metrics for unfound pup", u.PupID)
		return
	}
	p := t.state[u.PupID]

	for _, m := range p.Manifest.Metrics {
		val, ok := u.Payload[m.Name]
		if !ok {
			// no value for metric
			continue
		}

		switch m.Type {
		case "string":
			v, ok := val.Value.(string)
			if !ok {
				fmt.Printf("metric value for %s is not string", m.Name)
				continue
			}
			b := s.Metrics[m.Name].(*Buffer[string])
			b.Add(v)
		case "int":
			// convert various things to int..
			var vi int
			switch v := val.Value.(type) {
			case float32:
				vi = int(v)
			case float64:
				vi = int(v)
			case int:
				vi = v
			default:
				fmt.Printf("metric value for %s is not int", m.Name, reflect.TypeOf(val.Value))
				continue
			}
			b := s.Metrics[m.Name].(*Buffer[int])
			b.Add(vi)
		case "float":
			v, ok := val.Value.(float64)
			if !ok {
				fmt.Printf("metric value for %s is not float", m.Name)
				continue
			}
			b := s.Metrics[m.Name].(*Buffer[float64])
			b.Add(v)
		default:
			fmt.Println("Manifest metric unknown field type", m.Type)
		}
	}
}

// Modify provided pup to update warning flags
func (t PupManager) healthCheckPupState(pup *PupState) {
	// are our required config fields set?
	configSet := true
loop:
	for _, section := range pup.Manifest.Config.Sections {
		for _, field := range section.Fields {
			if field.Required {
				_, ok := pup.Config[field.Name]
				if !ok {
					configSet = false
					break loop
				}
			}
		}
	}

	// are our deps met?
	depsMet := true
	depsNotRunning := []string{}
	for _, d := range t.calculateDeps(pup) {
		depMet := false
		for iface, pupID := range pup.Providers {
			if d.Interface == iface {
				depMet = true
				provPup, ok := t.stats[pupID]
				if !ok {
					depMet = false
					fmt.Printf("pup %s missing, but provides %s to %s", pupID, iface, pup.ID)
				} else {
					if provPup.Status != STATE_RUNNING {
						depsNotRunning = append(depsNotRunning, iface)
					}
				}
			}
		}
		if !depMet {
			depsMet = false
		}
	}

	// Update pupState
	pup.NeedsConf = !configSet
	pup.NeedsDeps = !depsMet

	// Update pupStats
	issues := PupIssues{
		DepsNotRunning: depsNotRunning,
		// TODO: HealthWarnings
		// TODO: UpdateAvailable
	}
	t.stats[pup.ID].Issues = issues
}

// This function calculates a DependencyReport for every
// dep that a given pup requires
func (t PupManager) calculateDeps(pupState *PupState) []PupDependencyReport {
	deps := []PupDependencyReport{}
	for _, dep := range pupState.Manifest.Dependencies {
		report := PupDependencyReport{
			Interface: dep.InterfaceName,
			Version:   dep.InterfaceVersion,
		}

		constraint, err := semver.NewConstraint(dep.InterfaceVersion)
		if err != nil {
			fmt.Printf("Invalid version constraint: %s, %s:%s\n", pupState.Manifest.Meta.Name, dep.InterfaceName, dep.InterfaceVersion)
			deps = append(deps, report)
			continue
		}

		// Is there currently a provider set?
		report.CurrentProvider = pupState.Providers[dep.InterfaceName]

		// What are all installed pups that can provide the interface?
		installed := []string{}
		for id, p := range t.state {
			// search the interfaces and check against constraint
			for _, iface := range p.Manifest.Interfaces {
				ver, err := semver.NewVersion(iface.Version)
				if err != nil {
					continue
				}
				if iface.Name == dep.InterfaceName && constraint.Check(ver) == true {
					installed = append(installed, id)
				}
			}
		}
		report.InstalledProviders = installed

		// What are all available pups that can provide the interface?
		available := []pup.PupManifestDependencySource{}
		sourceList, err := t.sourceManager.GetAll(false)
		if err == nil {
			for _, list := range sourceList {
				// search the interfaces and check against constraint
				for _, p := range list.Pups {
					for _, iface := range p.Manifest.Interfaces {
						ver, err := semver.NewVersion(iface.Version)
						if err != nil {
							continue
						}
						if iface.Name == dep.InterfaceName && constraint.Check(ver) == true {
							// check if this isnt alread installed..
							alreadyInstalled := false
							for _, installedPupID := range installed {
								iPup, _, err := t.GetPup(installedPupID)
								if err != nil {
									continue
								}
								if iPup.Source.Location == list.Config.Location && iPup.Manifest.Meta.Name == p.Name {
									// matching location and name, assume already installed
									alreadyInstalled = true
									break
								}
							}

							if !alreadyInstalled {
								available = append(available, pup.PupManifestDependencySource{
									SourceLocation: list.Config.Location,
									PupName:        p.Name,
									PupVersion:     p.Version,
								})
							}
						}
					}
				}
			}
			report.InstallableProviders = available
		}

		// Is there a DefaultSourceProvider
		report.DefaultSourceProvider = dep.DefaultSource

		deps = append(deps, report)
	}
	return deps
}

func (t PupManager) CalculateDeps(pupID string) ([]PupDependencyReport, error) {
	pup, ok := t.state[pupID]
	if !ok {
		return []PupDependencyReport{}, errors.New("no such pup")
	}
	return t.calculateDeps(pup), nil
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

	t.updateMonitoredPups()
	return nil
}

func (t PupManager) indexPup(p *PupState) {
	s := PupStats{
		ID:          p.ID,
		Status:      STATE_STOPPED,
		StatCPU:     NewFloatBuffer(30),
		StatMEM:     NewFloatBuffer(30),
		StatMEMPERC: NewFloatBuffer(30),
		StatDISK:    NewFloatBuffer(30),
		Metrics:     map[string]interface{}{},
	}
	// handle custom metrics defined in manifest
	for _, m := range p.Manifest.Metrics {
		if m.Name == "" || m.HistorySize <= 0 {
			fmt.Println("Manifest metric has invalid fields", m)
			continue
		}
		switch m.Type {
		case "string":
			s.Metrics[m.Name] = NewBuffer[string](m.HistorySize)
		case "int":
			s.Metrics[m.Name] = NewBuffer[int](m.HistorySize)
		case "float":
			s.Metrics[m.Name] = NewBuffer[float64](m.HistorySize)
		default:
			fmt.Println("Manifest metric unknown field type", m.Type)
		}
	}

	t.state[p.ID] = p
	t.stats[p.ID] = &s
}

/* Set the list of monitored services on the SystemMonitor */
func (t PupManager) updateMonitoredPups() {
	serviceNames := []string{}
	for _, p := range t.state {
		if p.Installation == STATE_READY {
			serviceNames = append(serviceNames, fmt.Sprintf("container@pup-%s.service", p.ID))
		}
	}
	t.monitor.GetMonChannel() <- serviceNames
}

// called when we expect a pup to be changing state,
// this will rapidly poll for a few seconds and update
// the frontend with status.
func (t PupManager) FastPollPup(id string) {
	t.monitor.GetFastMonChannel() <- fmt.Sprintf("container@pup-%s.service", id)
}

func (t PupManager) GetPupSpecificEnvironmentVariablesForContainer(pupID string) map[string]string {
	env := map[string]string{
		"DBX_PUP_ID": pupID,
		"DBX_PUP_IP": t.state[pupID].IP,
	}

	// Iterate over each of our configured interfaces, and expose the host and port of each
	for _, iface := range t.state[pupID].Manifest.Dependencies {
		providerPup, ok := t.state[t.state[pupID].Providers[iface.InterfaceName]]
		if !ok {
			continue
		}

		interfaceName := toValidEnvKey(iface.InterfaceName)

		var providerPupExposes pup.PupManifestExposeConfig

	outer:
		for _, expose := range providerPup.Manifest.Container.Exposes {
			for _, exposeInterface := range expose.Interfaces {
				if exposeInterface == iface.InterfaceName {
					providerPupExposes = expose
					break outer
				}
			}
		}

		env["DBX_IFACE_"+interfaceName+"_NAME"] = providerPupExposes.Name
		env["DBX_IFACE_"+interfaceName+"_HOST"] = providerPup.IP
		env["DBX_IFACE_"+interfaceName+"_PORT"] = strconv.Itoa(providerPupExposes.Port)
	}

	return env
}

func toValidEnvKey(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return -1
	}, s)
	return strings.ToUpper(s)
}

/*****************************************************************************/
/*                Varadic Update Funcs for PupManager.UpdatePup:             */
/*****************************************************************************/

func SetPupInstallation(state string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Installation = state
		*pu = append(*pu, Pupdate{
			ID:    p.ID,
			Event: PUP_CHANGED_INSTALLATION,
			State: *p,
		})
	}
}

func SetPupConfig(newFields map[string]string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		for k, v := range newFields {
			p.Config[k] = v
		}
	}
}

func SetPupProviders(newProviders map[string]string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		for k, v := range newProviders {
			p.Providers[k] = v
		}
	}
}

func PupEnabled(b bool) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Enabled = b
	}
}
