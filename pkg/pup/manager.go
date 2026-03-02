package pup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/utils"
)

const (
	MIN_WEBUI_PORT int = 10000 // start assigning ports from..
)

/* The PupManager is collection of PupState and PupStats
* for all installed Pups.
*
* It supports subscribing to changes and ensures pups
* are persisted to disk.
 */

type PupManager struct {
	config            dogeboxd.ServerConfig
	pupDir            string // Where pup state is stored
	snapshotsDir      string // Where pup snapshots are stored
	lastIP            net.IP // last issued IP address
	lastPort          int    // last issued Port
	mu                *sync.Mutex
	state             map[string]*dogeboxd.PupState
	stats             map[string]*dogeboxd.PupStats
	updateSubscribers map[chan dogeboxd.Pupdate]bool    // listeners for 'Pupdates'
	statsSubscribers  map[chan []dogeboxd.PupStats]bool // listeners for 'PupStats'
	monitor           dogeboxd.SystemMonitor
	sourceManager     dogeboxd.SourceManager
	updateChecker     *UpdateChecker // Embedded update checker
}

func NewPupManager(config dogeboxd.ServerConfig, monitor dogeboxd.SystemMonitor) (*PupManager, error) {
	pupDir := filepath.Join(config.DataDir, "pups")

	if _, err := os.Stat(pupDir); os.IsNotExist(err) {
		log.Printf("Pup directory %q not found, creating it", pupDir)
		err = os.MkdirAll(pupDir, 0755)
		if err != nil {
			return &PupManager{}, fmt.Errorf("failed to create pup directory: %w", err)
		}
	}

	// Create snapshots directory
	snapshotsDir := filepath.Join(config.DataDir, "pup-snapshots")
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		log.Printf("Warning: failed to create snapshots directory: %v", err)
	}

	mu := sync.Mutex{}
	p := PupManager{
		config:            config,
		pupDir:            pupDir,
		snapshotsDir:      snapshotsDir,
		state:             map[string]*dogeboxd.PupState{},
		stats:             map[string]*dogeboxd.PupStats{},
		updateSubscribers: map[chan dogeboxd.Pupdate]bool{},
		statsSubscribers:  map[chan []dogeboxd.PupStats]bool{},
		mu:                &mu,
		monitor:           monitor,
	}
	// load pups from disk
	err := p.loadPups()
	if err != nil {
		return &p, err
	}

	// Recover any pups that were stuck in installing state. Sometimes this happens during development - for eg. if dogeboxd crashes during a pup installation
	p.recoverStuckPups()

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
	return &p, nil
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
						s.Status = derivePupStatusFromProc(*p, v)
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
						s.Status = derivePupStatusFromProc(*p, v)

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

func derivePupStatusFromProc(p dogeboxd.PupState, v dogeboxd.ProcStatus) dogeboxd.PupRunStatus {
	// Prefer systemd’s view when available, because MainPID can be 0 during transitions.
	switch v.ActiveState {
	case "activating":
		// If we're supposed to be enabled, we're starting; otherwise it's likely stabilizing after a stop.
		if p.Enabled {
			return dogeboxd.STATE_STARTING
		}
		return dogeboxd.STATE_STOPPING
	case "deactivating":
		return dogeboxd.STATE_STOPPING
	case "active":
		// If systemd says active but the user disabled the pup, show stopping.
		if !p.Enabled {
			return dogeboxd.STATE_STOPPING
		}
		// If enabled and systemd active, treat as running even if MainPID isn't resolvable.
		return dogeboxd.STATE_RUNNING
	}

	// Fallback to process presence + desired enabled state.
	if v.Running && p.Enabled {
		return dogeboxd.STATE_RUNNING
	}
	if v.Running && !p.Enabled {
		return dogeboxd.STATE_STOPPING
	}
	if !v.Running && p.Enabled {
		return dogeboxd.STATE_STARTING
	}
	return dogeboxd.STATE_STOPPED
}

func (t *PupManager) SetSourceManager(sourceManager dogeboxd.SourceManager) {
	t.sourceManager = sourceManager

	// Initialize update checker now that we have source manager
	if t.updateChecker == nil {
		t.updateChecker = NewUpdateChecker(t, sourceManager, t.config.DataDir)
	}
}

// Update checking methods - delegate to embedded UpdateChecker

func (t *PupManager) CheckForUpdates(pupID string) (dogeboxd.PupUpdateInfo, error) {
	if t.updateChecker == nil {
		return dogeboxd.PupUpdateInfo{}, fmt.Errorf("update checker not initialized")
	}
	return t.updateChecker.CheckForUpdates(pupID)
}

func (t *PupManager) CheckAllPupUpdates() map[string]dogeboxd.PupUpdateInfo {
	if t.updateChecker == nil {
		return make(map[string]dogeboxd.PupUpdateInfo)
	}
	return t.updateChecker.CheckAllPupUpdates()
}

func (t *PupManager) GetCachedUpdateInfo(pupID string) (dogeboxd.PupUpdateInfo, bool) {
	if t.updateChecker == nil {
		return dogeboxd.PupUpdateInfo{}, false
	}
	return t.updateChecker.GetCachedUpdateInfo(pupID)
}

func (t *PupManager) GetAllCachedUpdates() map[string]dogeboxd.PupUpdateInfo {
	if t.updateChecker == nil {
		return make(map[string]dogeboxd.PupUpdateInfo)
	}
	return t.updateChecker.GetAllCachedUpdates()
}

func (t *PupManager) ClearCacheEntry(pupID string) {
	if t.updateChecker != nil {
		t.updateChecker.ClearCacheEntry(pupID)
	}
}

func (t *PupManager) StartPeriodicCheck(stop chan bool) {
	if t.updateChecker != nil {
		t.updateChecker.StartPeriodicCheck(stop)
	}
}

func (t *PupManager) GetEventChannel() <-chan dogeboxd.PupUpdatesCheckedEvent {
	if t.updateChecker == nil {
		ch := make(chan dogeboxd.PupUpdatesCheckedEvent)
		close(ch)
		return ch
	}
	return t.updateChecker.GetEventChannel()
}

func (t *PupManager) DetectInterfaceChanges(oldManifest, newManifest dogeboxd.PupManifest) []dogeboxd.PupInterfaceVersion {
	if t.updateChecker == nil {
		return []dogeboxd.PupInterfaceVersion{}
	}
	return t.updateChecker.DetectInterfaceChanges(oldManifest, newManifest)
}

// StopPup stops a running pup by disabling it and triggering a rebuild
// This is safer than using _dbxroot pup stop directly as it ensures proper state management
func (t *PupManager) StopPup(pupID string, nixManager dogeboxd.NixManager, logger dogeboxd.SubLogger) error {
	// Get current pup state
	pup, _, err := t.GetPup(pupID)
	if err != nil {
		return fmt.Errorf("failed to get pup: %w", err)
	}

	if !pup.Enabled {
		// Already stopped
		return nil
	}

	// Disable the pup
	_, err = t.UpdatePup(pupID, dogeboxd.PupEnabled(false))
	if err != nil {
		return fmt.Errorf("failed to disable pup: %w", err)
	}

	// Rebuild to apply the change
	if err := nixManager.Rebuild(logger); err != nil {
		return fmt.Errorf("failed to rebuild after disabling pup: %w", err)
	}

	return nil
}

// StartPup starts a stopped pup by enabling it and triggering a rebuild.
// Note: This only updates the in-memory state and triggers a rebuild. For the container
// to actually start, the caller must ensure the nix pup file is written with Enabled=true
// before this is called. Prefer using enablePup in SystemUpdater which handles this correctly.
func (t *PupManager) StartPup(pupID string, nixManager dogeboxd.NixManager, logger dogeboxd.SubLogger) error {
	// Get current pup state
	pup, _, err := t.GetPup(pupID)
	if err != nil {
		return fmt.Errorf("failed to get pup: %w", err)
	}

	if pup.Enabled {
		// Already enabled
		return nil
	}

	// Enable the pup in memory
	_, err = t.UpdatePup(pupID, dogeboxd.PupEnabled(true))
	if err != nil {
		return fmt.Errorf("failed to enable pup: %w", err)
	}

	// Rebuild to apply the change
	if err := nixManager.Rebuild(logger); err != nil {
		return fmt.Errorf("failed to rebuild after enabling pup: %w", err)
	}

	return nil
}

/* Hand out channels to pupdate subscribers */
func (t PupManager) GetUpdateChannel() chan dogeboxd.Pupdate {
	ch := make(chan dogeboxd.Pupdate, 50)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.updateSubscribers[ch] = true
	return ch
}

/* Hand out channels to stat subscribers */
func (t PupManager) GetStatsChannel() chan []dogeboxd.PupStats {
	ch := make(chan []dogeboxd.PupStats, 50)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.statsSubscribers[ch] = true
	return ch
}

func (t PupManager) GetStateMap() map[string]dogeboxd.PupState {
	out := map[string]dogeboxd.PupState{}
	for k, v := range t.state {
		out[k] = *v
	}
	return out
}

func (t PupManager) GetStatsMap() map[string]dogeboxd.PupStats {
	out := map[string]dogeboxd.PupStats{}
	for k, v := range t.stats {
		out[k] = *v
	}
	return out
}

func (t PupManager) GetAssetsMap() map[string]dogeboxd.PupAsset {
	out := map[string]dogeboxd.PupAsset{}
	for k, v := range t.state {
		logos := dogeboxd.PupLogos{}

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

		out[k] = dogeboxd.PupAsset{
			Logos: logos,
		}
	}
	return out
}

func (t PupManager) GetPup(id string) (dogeboxd.PupState, dogeboxd.PupStats, error) {
	state, ok := t.state[id]
	if ok {
		return *state, *t.stats[id], nil
	}
	return dogeboxd.PupState{}, dogeboxd.PupStats{}, dogeboxd.ErrPupNotFound
}

func (t PupManager) FindPupByIP(ip string) (dogeboxd.PupState, dogeboxd.PupStats, error) {
	for _, p := range t.state {
		if ip == p.IP {
			return t.GetPup(p.ID)
		}
	}
	return dogeboxd.PupState{}, dogeboxd.PupStats{}, dogeboxd.ErrPupNotFound
}

func (t PupManager) GetAllFromSource(source dogeboxd.ManifestSourceConfiguration) []*dogeboxd.PupState {
	pups := []*dogeboxd.PupState{}

	for _, pup := range t.state {
		if pup.Source == source {
			pups = append(pups, pup)
		}
	}

	return pups
}

func (t PupManager) GetPupFromSource(name string, source dogeboxd.ManifestSourceConfiguration) *dogeboxd.PupState {
	for _, pup := range t.state {
		if pup.Source == source && pup.Manifest.Meta.Name == name {
			return pup
		}
	}
	return nil
}

// send pupdates to subscribers
func (t PupManager) sendPupdate(p dogeboxd.Pupdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Collect channels to remove (closed or full)
	toRemove := []chan dogeboxd.Pupdate{}

	for ch := range t.updateSubscribers {
		// Use recover to catch panics from closed channels
		func(ch chan dogeboxd.Pupdate) {
			defer func() {
				if r := recover(); r != nil {
					// Channel is closed, mark for removal
					toRemove = append(toRemove, ch)
				}
			}()
			select {
			case ch <- p:
				// sent pupdate to subscriber
			default:
				// channel is full, mark for removal
				toRemove = append(toRemove, ch)
			}
		}(ch)
	}

	// Remove closed/full channels
	for _, ch := range toRemove {
		delete(t.updateSubscribers, ch)
	}
}

// send stats to subscribers
func (t PupManager) sendStats() {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := []dogeboxd.PupStats{}

	for _, v := range t.stats {
		stats = append(stats, *v)
	}

	// Collect channels to remove (closed or full)
	toRemove := []chan []dogeboxd.PupStats{}

	for ch := range t.statsSubscribers {
		// Use recover to catch panics from closed channels
		func(ch chan []dogeboxd.PupStats) {
			defer func() {
				if r := recover(); r != nil {
					// Channel is closed, mark for removal
					toRemove = append(toRemove, ch)
				}
			}()
			select {
			case ch <- stats:
				// sent stats to subscriber
			default:
				// channel is full, mark for removal
				toRemove = append(toRemove, ch)
			}
		}(ch)
	}

	// Remove closed/full channels
	for _, ch := range toRemove {
		delete(t.statsSubscribers, ch)
	}
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

		var providerPupExposes dogeboxd.PupManifestExposeConfig

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

// recoverStuckPups checks for pups that were stuck in "installing" state - mark them as broken
func (t *PupManager) recoverStuckPups() {
	for id, pup := range t.state {
		if pup.Installation == dogeboxd.STATE_INSTALLING {
			_, err := t.UpdatePup(id, dogeboxd.SetPupInstallation(dogeboxd.STATE_BROKEN), dogeboxd.SetPupBrokenReason(dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED))
			if err != nil {
				log.Printf("Failed to mark pup %s as broken: %v", id, err)
			}
		}
	}
}

// Snapshot management methods

// getSnapshotFilePath returns the path to a specific pup's snapshot file
func (t *PupManager) getSnapshotFilePath(pupID string) string {
	return filepath.Join(t.snapshotsDir, fmt.Sprintf("%s.json", pupID))
}

// CreateSnapshot creates a snapshot of the current pup state before an upgrade
func (t *PupManager) CreateSnapshot(pupState dogeboxd.PupState) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := dogeboxd.PupVersionSnapshot{
		Version:        pupState.Version,
		Manifest:       pupState.Manifest,
		Config:         pupState.Config,
		Providers:      pupState.Providers,
		Enabled:        pupState.Enabled,
		SnapshotDate:   time.Now(),
		SourceID:       pupState.Source.ID,
		SourceLocation: pupState.Source.Location,
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	filePath := t.getSnapshotFilePath(pupState.ID)

	// Write to temp file first, then rename for atomicity
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename snapshot file: %w", err)
	}

	log.Printf("Created upgrade snapshot for pup %s (version %s)", pupState.ID, pupState.Version)
	return nil
}

// GetSnapshot retrieves a pup's version snapshot if it exists
func (t *PupManager) GetSnapshot(pupID string) (*dogeboxd.PupVersionSnapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	filePath := t.getSnapshotFilePath(pupID)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No snapshot exists
		}
		return nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	var snapshot dogeboxd.PupVersionSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot file: %w", err)
	}

	return &snapshot, nil
}

// HasSnapshot checks if a snapshot exists for a pup
func (t *PupManager) HasSnapshot(pupID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	filePath := t.getSnapshotFilePath(pupID)
	_, err := os.Stat(filePath)
	return err == nil
}

// DeleteSnapshot removes a pup's version snapshot
func (t *PupManager) DeleteSnapshot(pupID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	filePath := t.getSnapshotFilePath(pupID)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted, not an error
		}
		return fmt.Errorf("failed to delete snapshot file: %w", err)
	}

	log.Printf("Deleted upgrade snapshot for pup %s", pupID)
	return nil
}

// ListSnapshots returns a list of all pup IDs that have snapshots
func (t *PupManager) ListSnapshots() ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries, err := os.ReadDir(t.snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	var pupIDs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			pupIDs = append(pupIDs, name[:len(name)-5]) // Remove .json extension
		}
	}

	return pupIDs, nil
}

// CleanOldSnapshots removes snapshots older than the specified duration
func (t *PupManager) CleanOldSnapshots(maxAge time.Duration) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries, err := os.ReadDir(t.snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	cleanedCount := 0
	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(t.snapshotsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var snapshot dogeboxd.PupVersionSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		if snapshot.SnapshotDate.Before(cutoff) {
			if err := os.Remove(filePath); err == nil {
				cleanedCount++
				log.Printf("Cleaned old snapshot: %s (created %s)", entry.Name(), snapshot.SnapshotDate)
			}
		}
	}

	return cleanedCount, nil
}
