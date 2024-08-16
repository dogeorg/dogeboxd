/*
Dogebox internal architecture:

 Actions are instructions from the user to do something, and come externally
 via the REST API or Websocket etc.  These are submitted to Dogeboxd.AddAction
 and become Jobs in the job queue, returning a Job ID

 Jobs are either processed directly, or if related to the system in some way,
 handed to the SystemUpdater.

 Completed Jobs are submitted to the Changes channel for reporting back to
 the user, along with their Job ID.

                                       ┌──────────────┐
                                       │  Dogeboxd{}  │
                                       │              │
                                       │  ┌────────►  │
                                       │  │Dogebox │  │
 REST API  ─────┐                      │  │Run Loop│  │
                │                      │  ◄──────┬─┘  │
                │                      │     ▲   │    │
                │                      │     │   ▼    │
                │              ======= │  ┌──┴─────►  │ =======   Job ID
                │ Actions      Jobs    │  │ System │  │ Changes
 WebSocket ─────┼───────────►  Channel │  │ Updater│  │ Channel ───► WebSocket
                │ Job ID       ======= │  ◄────────┘  │ =======
                │ ◄────                │              │
                │                      │   ▲      │   │
                │                      │   │      │   │
                │                      └───┼──────┼───┘
 System         │                          │      │
 Events   ──────┘                          │      ▼
                                           Nix CLI
                                           SystemD

*/

package dogeboxd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
)

// A job is an in-flight handling of an Action (see actions.go)
type Job struct {
	A        Action
	ID       string
	Err      string
	Success  any
	Manifest *PupManifest // nilable, check before use!
	Status   *PupStatus   // nilable, check before use!
}

// A Change is the result of a Job that will be sent back
// to the Job issuer.
type Change struct {
	ID     string `json:"id"`
	Error  string `json:"error"`
	Type   string `json:"type"`
	Update any    `json:"update"`
}

type InternalState struct {
	ActionCounter int
	InstalledPups []string
}

type Dogeboxd struct {
	Manifests      ManifestIndex
	Pups           map[string]PupStatus
	Internal       *InternalState
	SystemUpdater  SystemUpdater
	SystemMonitor  SystemMonitor
	JournalReader  JournalReader
	NetworkManager NetworkManager
	lifecycle      LifecycleManager
	jobs           chan Job
	Changes        chan Change
}

func NewDogeboxd(pupDir string, man ManifestIndex, updater SystemUpdater, monitor SystemMonitor, journal JournalReader, networkManager NetworkManager, lifecycle LifecycleManager) Dogeboxd {
	intern := InternalState{
		ActionCounter: 100000,
		InstalledPups: []string{"internal.dogeboxd"},
	}
	// TODO: Load state from GOB
	s := Dogeboxd{
		Manifests:      man,
		Pups:           map[string]PupStatus{},
		SystemUpdater:  updater,
		SystemMonitor:  monitor,
		JournalReader:  journal,
		NetworkManager: networkManager,
		lifecycle:      lifecycle,
		jobs:           make(chan Job),
		Changes:        make(chan Change),
		Internal:       &intern,
	}

	// TODO start monitoring all installed services
	monitor.GetMonChannel() <- []string{"dbus.service"}

	// load pup state from disk
	for _, ip := range s.Internal.InstalledPups {
		fmt.Printf("Loading pup status: %s\n", ip)
		m, ok := s.Manifests.FindManifest(ip)
		if !ok {
			fmt.Printf("Failed to load %s, no matching manifest source\n", ip)
			continue
		}
		s.loadPupStatus(pupDir, m)
	}
	return s
}

// Main Dogeboxd goroutine, handles routing messages in
// and out of the system via job and change channels,
// handles messages from subsystems ie: SystemUpdater,
// SysteMonitor etc.
func (t Dogeboxd) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
			dance:
				select {

				// Handle shutdown
				case <-stop:
					break mainloop

				// Hand incomming jobs to the Job Dispatcher
				case j, ok := <-t.jobs:
					if !ok {
						break dance
					}
					t.jobDispatcher(j)

				// Handle completed jobs from SystemUpdater
				case j, ok := <-t.SystemUpdater.GetUpdateChannel():
					if !ok {
						break dance
					}
					t.sendFinishedJob("system", j)

				// Handle updates from the system monitor
				case v, ok := <-t.SystemMonitor.GetStatChannel():
					if !ok {
						break dance
					}
					t.Changes <- Change{"", "", "status", v}
				}
			}
		}()
		// flag to Conductor we are running
		started <- true
		// Wait on a stop signal
		<-stop
		// do shutdown things and flag we are stopped
		stopped <- true
	}()
	return nil
}

/* jobDispatcher handles any incomming Jobs
 * based on their Action type, some to internal
 * helpers and others sent to the system updater
 * for handling.
 */
func (t Dogeboxd) jobDispatcher(j Job) {
	fmt.Printf("dispatch job %+v\n", j)
	switch a := j.A.(type) {
	// System actions
	case InstallPup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case UninstallPup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case EnablePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case DisablePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)

	// Dogebox actions
	case UpdatePupConfig:
		t.updatePupConfig(j, a)

	// Host Actions
	case UpdatePendingSystemNetwork:
		t.SystemUpdater.AddJob(j)

	default:
		fmt.Printf("Unknown action type: %v\n", a)
	}
}

// Add an Action to the Action queue, returns a unique ID
// which can be used to match the outcome in the Event queue
func (t Dogeboxd) AddAction(a Action) string {
	t.Internal.ActionCounter++
	id := fmt.Sprintf("%x", t.Internal.ActionCounter)
	t.jobs <- Job{A: a, ID: id}
	return id
}

// Flatten the manifest sources for the frontend API
func (t Dogeboxd) GetManifests() map[string]ManifestSourceExport {
	return t.Manifests.GetManifestMap()
}

// Get all pup stats for the frontend API
func (t Dogeboxd) GetPupStats() map[string]PupStatus {
	return t.Pups
}

// Get a log channel for a specific pup for the websocket API
func (t Dogeboxd) GetLogChannel(pupID string) (context.CancelFunc, chan string, error) {
	// find the manifest, get the systemd service-name,
	// subscribe to the JournalReader for that service:
	man, ok := t.Manifests.FindManifest(pupID)
	if !ok {
		return nil, nil, errors.New("PUP not found")
	}
	// TODO this should possibly be the responsibility off
	// journal reader so systemd concepts dont bleed into
	// dogecoind..
	service := fmt.Sprintf("%s.service", man.ID)
	fmt.Println("conencting to systemd journal: ", service)
	service = "dbus.service" // TODO HAX REMOVE
	return t.JournalReader.GetJournalChan(service)
}

// helper to report a completed job back to the client
func (t Dogeboxd) sendFinishedJob(changeType string, j Job) {
	t.Changes <- Change{ID: j.ID, Error: j.Err, Type: changeType, Update: j.Success}
}

// create or load PupStatus for a given PUP id
func (t *Dogeboxd) loadPupStatus(pupDir string, m PupManifest) {
	p := NewPUPStatus(pupDir, m)
	p.Read()
	t.Pups[p.ID] = p
}

func (t Dogeboxd) setJobPupDetails(j *Job, PupID string) error {
	m, ok := t.Manifests.FindManifest(PupID)
	if !ok {
		return errors.New(fmt.Sprintf("couldnt find manifest %s", PupID))
	}
	j.Manifest = &m
	return nil
}

func (t Dogeboxd) sendSystemJobWithPupDetails(j Job, PupID string) {
	err := t.setJobPupDetails(&j, PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("system", j)
		return
	}
	// Send job to the system updater for handling
	t.SystemUpdater.AddJob(j)
}

// Handle an UpdatePupConfig action
func (t *Dogeboxd) updatePupConfig(j Job, u UpdatePupConfig) {
	p, ok := t.Pups[u.PupID]
	if !ok {
		j.Err = fmt.Sprintf("Couldnt find pup to update: %s", u.PupID)
		t.sendFinishedJob("PupStatus", j)
		return
	}

	old := p.Config
	newConfig := map[string]string{}

	for k, v := range old {
		newConfig[k] = v
	}

	// TODO validate against manifest fields
	for k, v := range u.Payload {
		newConfig[k] = v
	}
	p.Config = newConfig

	err := p.Write()
	if err != nil {
		j.Err = fmt.Sprintf("Failed to write Pup state to disk %v", err)
		p.Config = old
		t.sendFinishedJob("PupStatus", j)
		return
	}
	j.Success = p
	t.Pups[u.PupID] = p
	t.sendFinishedJob("PupStatus", j)
}
