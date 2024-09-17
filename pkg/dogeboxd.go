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
	"crypto/rand"
	_ "embed"
	"fmt"
	"log"
)

type Dogeboxd struct {
	Pups           PupManager
	SystemUpdater  SystemUpdater
	SystemMonitor  SystemMonitor
	JournalReader  JournalReader
	NetworkManager NetworkManager
	sm             StateManager
	sources        SourceManager
	nix            NixManager
	jobs           chan Job
	Changes        chan Change
}

func NewDogeboxd(
	stateManager StateManager,
	pups PupManager,
	updater SystemUpdater,
	monitor SystemMonitor,
	journal JournalReader,
	networkManager NetworkManager,
	sourceManager SourceManager,
	nixManager NixManager,
) Dogeboxd {
	s := Dogeboxd{
		Pups:           pups,
		SystemUpdater:  updater,
		SystemMonitor:  monitor,
		JournalReader:  journal,
		NetworkManager: networkManager,
		sm:             stateManager,
		sources:        sourceManager,
		nix:            nixManager,
		jobs:           make(chan Job),
		Changes:        make(chan Change),
	}

	return s
	// TODO start monitoring all installed services
	// SUB TO PUP MANAGER monitor.GetMonChannel() <- []string{"dbus.service"}
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

				// Handle pupdates from PupManager
				case p, ok := <-t.Pups.GetUpdateChannel():
					if !ok {
						break dance
					}
					t.Changes <- Change{"internal", "", "pup", p.State}

				// Handle stats from PupManager
				case stats, ok := <-t.Pups.GetStatsChannel():
					if !ok {
						break dance
					}
					t.Changes <- Change{"internal", "", "stats", stats}

				// Handle completed jobs from SystemUpdater
				case j, ok := <-t.SystemUpdater.GetUpdateChannel():
					if !ok {
						break dance
					}
					// if this job was successful, AND it was a
					// job that results in the stop/start of a pup,
					// tell the PupManager to poll for state changes
					switch j.A.(type) {
					case InstallPup:
						t.Pups.FastPollPup(j.State.ID)
					case EnablePup:
						t.Pups.FastPollPup(j.State.ID)
					case DisablePup:
						t.Pups.FastPollPup(j.State.ID)
					}

					// TODO: explain why we I this
					if j.Err == "" && j.State != nil {
						state, _, err := t.Pups.GetPup(j.State.ID)
						if err == nil {
							j.Success = state
						}
					}
					t.sendFinishedJob("action", j)

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

// Add an Action to the Action queue, returns a unique ID
// which can be used to match the outcome in the Event queue
func (t Dogeboxd) AddAction(a Action) string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Entropic Failure, add more Overminds.")
	}
	id := fmt.Sprintf("%x", b)
	t.jobs <- Job{A: a, ID: id}
	return id
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
		t.createPupFromManifest(j, a.PupName, a.PupVersion, a.SourceId)
	case UninstallPup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case PurgePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case EnablePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case DisablePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)

	// Dogebox actions
	case UpdatePupConfig:
		t.updatePupConfig(j, a)

	case UpdatePupProviders:
		t.updatePupProviders(j, a)

	// Host Actions
	case UpdatePendingSystemNetwork:
		t.SystemUpdater.AddJob(j)

	// Pup router actions
	case UpdateMetrics:
		t.Pups.UpdateMetrics(a)

	default:
		fmt.Printf("Unknown action type: %v\n", a)
	}
}

/* This is where we create a 'PupState' from a ManifestID
* and set it to be installed by the SystemUpdater. After
* this point the Pup has entered a managed state and will
* only be installable again after this one has been purged.
*
* Future: support multiple pup instances per manifest
 */
func (t *Dogeboxd) createPupFromManifest(j Job, pupName, pupVersion, sourceId string) {
	// Fetch the correct manifest from the source manager
	manifest, source, err := t.sources.GetSourceManifest(sourceId, pupName, pupVersion)
	if err != nil {
		j.Err = fmt.Sprintf("Couldn't create pup, no manifest: %s", err)
		t.sendFinishedJob("action", j)
		log.Println(j.Err)
		return
	}

	// create a new pup for the manifest
	pupID, err := t.Pups.AdoptPup(manifest, source)
	if err != nil {
		j.Err = fmt.Sprintf("Couldn't create pup: %s", err)
		t.sendFinishedJob("action", j)
		log.Println(j.Err)
		return
	}

	// send the job off to the SystemUpdater to install
	t.sendSystemJobWithPupDetails(j, pupID)
}

// Handle an UpdatePupConfig action
func (t *Dogeboxd) updatePupConfig(j Job, u UpdatePupConfig) {
	_, err := t.Pups.UpdatePup(u.PupID, SetPupConfig(u.Payload))
	if err != nil {
		fmt.Println("couldn't update pup", err)
		j.Err = fmt.Sprintf("Couldnt update: %s", u.PupID)
		t.sendFinishedJob("action", j)
		return
	}

	j.Success, _, err = t.Pups.GetPup(u.PupID)
	if err != nil {
		fmt.Println("Couldnt get pup", u.PupID)
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}
	t.sendFinishedJob("action", j)
}

// Handle an UpdatePupProviders action
func (t *Dogeboxd) updatePupProviders(j Job, u UpdatePupProviders) {
	_, err := t.Pups.UpdatePup(u.PupID, SetPupProviders(u.Payload))
	if err != nil {
		fmt.Println("couldn't update pup", err)
		j.Err = fmt.Sprintf("Couldnt update: %s", u.PupID)
		t.sendFinishedJob("action", j)
		return
	}

	j.Success, _, err = t.Pups.GetPup(u.PupID)
	if err != nil {
		fmt.Println("Couldnt get pup", u.PupID)
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	// Once we've updated our providers, we might need to rebuild
	// some of our container configurations to fix up firewall rules.
	if err := t.nix.UpdateSystemContainerConfiguration(); err != nil {
		fmt.Println("Failed to update container configuration:", err)
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	if err := t.nix.Rebuild(); err != nil {
		fmt.Println("Failed to rebuild:", err)
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	t.sendFinishedJob("action", j)
}

// helper to report a completed job back to the client
func (t Dogeboxd) sendFinishedJob(changeType string, j Job) {
	t.Changes <- Change{ID: j.ID, Error: j.Err, Type: changeType, Update: j.Success}
}

// helper to attach PupState to a job and send it to the SystemUpdater
func (t Dogeboxd) sendSystemJobWithPupDetails(j Job, PupID string) {
	p, _, err := t.Pups.GetPup(PupID)
	if err != nil {
		j.Err = err.Error()
		fmt.Println("Failed to get pup:", err)
		t.sendFinishedJob("action", j)
		return
	}
	j.State = &p

	// Send job to the system updater for handling
	t.SystemUpdater.AddJob(j)
}

// TODO: Shound not be on Dogeboxd, needs moving

// Get a log channel for a specific pup for the websocket API
func (t Dogeboxd) GetLogChannel(pupID string) (context.CancelFunc, chan string, error) {
	// find the manifest, get the systemd service-name,
	// subscribe to the JournalReader for that service:
	state, _, err := t.Pups.GetPup(pupID)
	if err != nil {
		return nil, nil, err
	}
	// TODO this should possibly be the responsibility off
	// journal reader so systemd concepts dont bleed into
	// dogecoind..
	service := fmt.Sprintf("%s.service", state.ID)
	fmt.Println("conencting to systemd journal: ", service)
	service = "dbus.service" // TODO HAX REMOVE
	return t.JournalReader.GetJournalChan(service)
}
