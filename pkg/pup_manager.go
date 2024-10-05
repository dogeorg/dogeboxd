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
	"github.com/dogeorg/dogeboxd/pkg/utils"
)

const (
	PUP_CHANGED_INSTALLATION int = iota
	PUP_ADOPTED                  = iota
)

const (
	MIN_WEBUI_PORT int = 10000 // start assigning ports from..
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
	lastPort          int    // last issued Port
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

	// Create any WebUIs listed as exposed
	uis := []PupWebUI{}
	for _, ex := range m.Container.Exposes {
		if ex.WebUI {
			uis = append(uis, PupWebUI{
				Name:     ex.Name,
				Internal: ex.Port,
			})
		}
	}

	// and give them all Ports
	if len(uis) > 0 {
		ports := t.nextAvailablePorts(len(uis))
		for i := range uis {
			uis[i].Port = ports[0]
			ports = ports[1:]
		}
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
		WebUIs:       uis,
	}

	// Now save it to disk
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
	// Done! Adpoted
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

// get N available webUI ports. These must be set on
// a PupState before you can call again without getting
// duplicates
func (t PupManager) nextAvailablePorts(howMany int) []int {
	if howMany <= 0 {
		return []int{}
	}
	out := []int{}
	consumed := map[int]struct{}{} // track already used ports

	// find all current ports
	for _, ps := range t.state {
		// any ports already assigned to WebUIs
		for _, w := range ps.WebUIs {
			consumed[w.Port] = struct{}{}
		}
		// and any ports Exposed by the manifest on the host
		for _, ex := range ps.Manifest.Container.Exposes {
			if ex.ListenOnHost {
				consumed[ex.Port] = struct{}{}
			}
		}
	}

	for len(out) < howMany {
		fmt.Println("filled ports", len(out))
		for port := MIN_WEBUI_PORT; true; port++ {
			fmt.Println("PORT", port)
			// check port not in use anywhere
			_, exists := consumed[port]
			if !exists {
				out = append(out, port)
				break
			}
		}
	}
	fmt.Println("sending needed ports", out)
	return out
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

func (t PupManager) GetAssetsMap() map[string]PupAsset {
	out := map[string]PupAsset{}
	for k, v := range t.state {
		logos := PupLogos{}

		if v.Manifest.Meta.LogoPath != "" {
			logoPath := filepath.Join(t.pupDir, k, v.Manifest.Meta.LogoPath)
			logoBytes, err := os.ReadFile(logoPath)
			if err == nil {
				logoBase64, err := utils.ImageBytesToWebBase64(logoBytes, v.Manifest.Meta.LogoPath)
				if err == nil {
					logos.MainLogoBase64 = logoBase64
				}
			}
		}

		out[k] = PupAsset{
			Logos: logos,
		}
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

						for _, m := range s.SystemMetrics {
							switch m.Name {
							case "CPU":
								m.Values.Add(v.CPUPercent)
							case "Memory":
								m.Values.Add(v.MEMMb)
							case "MemoryPercent":
								m.Values.Add(v.MEMPercent)
							case "DiskUsage":
								m.Values.Add(float64(0.0)) // TODO
							}
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
	for _, metric := range s.Metrics {
		metrics[metric.Name] = metric.Values.GetValues()
	}

	return metrics
}

func (t PupManager) addMetricValue(stats *PupStats, name string, value any) {
	for _, m := range stats.Metrics {
		if m.Name == name {
			m.Values.Add(value)
		}
	}
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
			t.addMetricValue(s, m.Name, v)
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
				fmt.Printf("metric value for %s is not int: %s", m.Name, reflect.TypeOf(val.Value))
				continue
			}
			t.addMetricValue(s, m.Name, vi)
		case "float":
			v, ok := val.Value.(float64)
			if !ok {
				fmt.Printf("metric value for %s is not float", m.Name)
				continue
			}
			t.addMetricValue(s, m.Name, v)
		default:
			fmt.Println("Manifest metric unknown field type", m.Type)
		}
	}
}

// This function only checks pup-specific conditions, it does not check
// the rest of the system is ready for a pup to start.
func (t PupManager) CanPupStart(pupId string) (bool, error) {
	pup, ok := t.state[pupId]
	if !ok {
		return false, errors.New("no such pup")
	}

	report := t.GetPupHealthState(pup)

	// If we still need config or deps, don't start.
	if report.NeedsConf || report.NeedsDeps {
		return false, nil
	}

	// TODO: This doesn't work when being called from our dbx CLI
	//       as our system updates aren't running.

	// If a dep isn't running, don't start.
	// if len(report.Issues.DepsNotRunning) > 0 {
	// 	return false, nil
	// }

	return true, nil
}

type PupHealthStateReport struct {
	Issues    PupIssues
	NeedsConf bool
	NeedsDeps bool
}

func (t PupManager) GetPupHealthState(pup *PupState) PupHealthStateReport {
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

	report := PupHealthStateReport{
		Issues: PupIssues{
			DepsNotRunning: depsNotRunning,
			// TODO: HealthWarnings
			// TODO: UpdateAvailable
		},
		NeedsConf: !configSet,
		NeedsDeps: !depsMet,
	}

	return report
}

// Modify provided pup to update warning flags
func (t PupManager) healthCheckPupState(pup *PupState) {
	report := t.GetPupHealthState(pup)

	pup.NeedsConf = report.NeedsConf
	pup.NeedsDeps = report.NeedsDeps
	t.stats[pup.ID].Issues = report.Issues
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
									PupLogoBase64:  p.LogoBase64,
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
	systemMetrics := []PupMetrics[any]{
		{
			Name:   "CPU",
			Label:  "CPU",
			Type:   "float",
			Values: NewBuffer[any](30),
		},
		{
			Name:   "Memory",
			Label:  "Memory",
			Type:   "float",
			Values: NewBuffer[any](30),
		},
		{
			Name:   "Memory Percent",
			Label:  "Memory Percent",
			Type:   "float",
			Values: NewBuffer[any](30),
		},
		{
			Name:   "Disk Usage",
			Label:  "Disk Usage",
			Type:   "float",
			Values: NewBuffer[any](30),
		},
	}

	metrics := []PupMetrics[any]{}

	// handle custom metrics defined in manifest
	for _, m := range p.Manifest.Metrics {
		if m.Name == "" || m.HistorySize <= 0 {
			fmt.Println("Manifest metric has invalid fields", m)
			continue
		}

		metric := PupMetrics[any]{
			Name:   m.Name,
			Label:  m.Label,
			Type:   m.Type,
			Values: NewBuffer[any](m.HistorySize),
		}

		metrics = append(metrics, metric)
	}

	s := PupStats{
		ID:            p.ID,
		Status:        STATE_STOPPED,
		SystemMetrics: systemMetrics,
		Metrics:       metrics,
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

func SetPupBrokenReason(reason string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.BrokenReason = reason
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
		if p.Providers == nil {
			p.Providers = make(map[string]string)
		}

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

func SetPupHooks(newHooks []PupHook) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		if p.Hooks == nil {
			p.Hooks = []PupHook{}
		}

		for _, hook := range newHooks {
			id, err := newID(16)
			if err != nil {
				fmt.Println("couldn't generate random ID for hook")
				continue
			}
			hook.ID = id
			p.Hooks = append(p.Hooks, hook)
		}
	}
}

// Generate a somewhat random ID string
func newID(l int) (string, error) {
	var ID string
	b := make([]byte, l)
	_, err := rand.Read(b)
	if err != nil {
		return ID, err
	}
	return fmt.Sprintf("%x", b), nil
}
