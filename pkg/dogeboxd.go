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
 WebSocket ─────┼───────────►  Channel │  │ Updater │  │ Channel ───► WebSocket
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
	"fmt"
)

// A job is an in-flight handling of an Action (see actions.go)
type Job struct {
	A       Action
	ID      string
	Err     string
	Success any
}

// A Change is the result of a Job that will be sent back
// to the Job issuer.
type Change struct {
	ID     string `json:"id"`
	Error  string `json:"error"`
	Type   string `json:"type"`
	Update any    `json:"update"`
}

type Dogeboxd struct {
	Manifests     ManifestIndex
	Pups          map[string]PupStatus
	Internal      *InternalState
	SystemUpdater SystemUpdater
	SystemMonitor SystemMonitor
	jobs          chan Job
	Changes       chan Change
}

func NewDogeboxd(pupDir string, man ManifestIndex, j SystemUpdater, m SystemMonitor) Dogeboxd {
	intern := InternalState{
		ActionCounter: 100000,
		InstalledPups: []string{"internal.dogeboxd"},
	}
	// TODO: Load state from GOB
	s := Dogeboxd{
		Manifests:     man,
		Pups:          map[string]PupStatus{},
		SystemUpdater: j,
		SystemMonitor: m,
		jobs:          make(chan Job),
		Changes:       make(chan Change),
		Internal:      &intern,
	}

	m.GetMonChannel() <- []string{"dbus.service"}
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

func (t Dogeboxd) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
			dance:
				select {
				case <-stop:
					break mainloop
				case v, ok := <-t.jobs:
					if !ok {
						break dance // ヾ(。⌒∇⌒)ノ
					}
					switch a := v.A.(type) {
					// case InstallPup, UninstallPup, StartPup, StopPup, RestartPup:
					case InstallPup:
						fmt.Printf("job: %+v\n\n", v)
						// These jobs are sent to the system manager to handle
						m, ok := t.Manifests.FindManifest(a.PupID)
						if !ok {
							fmt.Println("couldnt find manifest for pup action", a.PupID)
						}
						a.M = &m // add the Manifest to the action before passing along
						v.A = a
						t.SystemUpdater.AddJob(v)
					case LoadLocalPup:
						fmt.Println("Load local pup from ", a.Path)
					case UpdatePupConfig:
						fmt.Printf("Update pup config %v\n", a)
						err := t.updatePupConfig(&v, a)
						if err != nil {
							fmt.Println(err)
						}
						t.sendChange("PupStatus", v)
					default:
						fmt.Printf("Unknown action type: %v\n", a)
					}
				case v, ok := <-t.SystemUpdater.GetUpdateChannel():
					// Handle completed jobs coming back from the SystemUpdater
					// by sending them off via our Changes channel
					if !ok {
						break dance
					}

					t.Changes <- Change{v.ID, v.Err, "system", v.Success}
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

// Add an Action to the Action queue, returns a unique ID
// which can be used to match the outcome in the Event queue
func (t Dogeboxd) AddAction(a Action) string {
	t.Internal.ActionCounter++
	id := fmt.Sprintf("%x", t.Internal.ActionCounter)
	t.jobs <- Job{A: a, ID: id}
	return id
}

func (t Dogeboxd) sendChange(changeType string, j Job) {
	fmt.Println("SENDING CHANGE")
	t.Changes <- Change{ID: j.ID, Error: j.Err, Type: changeType, Update: j.Success}
}

func (t Dogeboxd) GetManifests() map[string]ManifestSourceExport {
	return t.Manifests.GetManifestMap()
}

func (t Dogeboxd) GetPupStats() map[string]PupStatus {
	return t.Pups
}

// create or load PupStatus for a given PUP id
func (t *Dogeboxd) loadPupStatus(pupDir string, m PupManifest) {
	fmt.Println("LOADING PUP STATUS")
	p := NewPUPStatus(pupDir, m)
	p.Read()
	t.Pups[p.ID] = p
}

func (t *Dogeboxd) updatePupConfig(j *Job, u UpdatePupConfig) error {
	p, ok := t.Pups[u.PupID]
	if !ok {
		j.Err = fmt.Sprintf("Couldnt find pup to update: %s", u.PupID)
		return fmt.Errorf(j.Err)
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
		return fmt.Errorf(j.Err)
	}
	j.Success = p
	t.Pups[u.PupID] = p
	return nil
}

// InternalState is stored in dogeboxd.gob and contains
// various details about what's installed, what condition
// we're in overall etc.
type InternalState struct {
	ActionCounter int
	InstalledPups []string
}
