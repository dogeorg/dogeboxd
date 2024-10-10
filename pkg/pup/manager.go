package pup

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/utils"
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
	pupDir            string // Where pup state is stored
	tmpDir            string // Where temporary files are stored
	lastIP            net.IP // last issued IP address
	lastPort          int    // last issued Port
	mu                *sync.Mutex
	state             map[string]*dogeboxd.PupState
	stats             map[string]*dogeboxd.PupStats
	updateSubscribers map[chan dogeboxd.Pupdate]bool    // listeners for 'Pupdates'
	statsSubscribers  map[chan []dogeboxd.PupStats]bool // listeners for 'PupStats'
	monitor           dogeboxd.SystemMonitor
	sourceManager     dogeboxd.SourceManager
}

func NewPupManager(dataDir string, tmpDir string, monitor dogeboxd.SystemMonitor) (*PupManager, error) {
	pupDir := filepath.Join(dataDir, "pups")

	if _, err := os.Stat(pupDir); os.IsNotExist(err) {
		log.Printf("Pup directory %q not found, creating it", pupDir)
		err = os.MkdirAll(pupDir, 0755)
		if err != nil {
			return &PupManager{}, fmt.Errorf("failed to create pup directory: %w", err)
		}
	}

	mu := sync.Mutex{}
	p := PupManager{
		pupDir:            pupDir,
		tmpDir:            tmpDir,
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
						if v.Running && p.Enabled {
							s.Status = dogeboxd.STATE_RUNNING
						} else if v.Running && !p.Enabled {
							s.Status = dogeboxd.STATE_STOPPING
						} else if !v.Running && p.Enabled {
							s.Status = dogeboxd.STATE_STARTING
						} else {
							s.Status = dogeboxd.STATE_STOPPED
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
							s.Status = dogeboxd.STATE_RUNNING
						} else if v.Running && !p.Enabled {
							s.Status = dogeboxd.STATE_STOPPING
						} else if !v.Running && p.Enabled {
							s.Status = dogeboxd.STATE_STARTING
						} else {
							s.Status = dogeboxd.STATE_STOPPED
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

func (t *PupManager) SetSourceManager(sourceManager dogeboxd.SourceManager) {
	t.sourceManager = sourceManager
}

/* Hand out channels to pupdate subscribers */
func (t PupManager) GetUpdateChannel() chan dogeboxd.Pupdate {
	ch := make(chan dogeboxd.Pupdate)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.updateSubscribers[ch] = true
	return ch
}

/* Hand out channels to stat subscribers */
func (t PupManager) GetStatsChannel() chan []dogeboxd.PupStats {
	ch := make(chan []dogeboxd.PupStats)
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

// send stats to subscribers
func (t PupManager) sendStats() {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := []dogeboxd.PupStats{}

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
