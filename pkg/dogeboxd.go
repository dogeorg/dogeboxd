package dogeboxd

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

//go:embed pup.json
var dbxManifestFile []byte

type Dogeboxd struct {
	Manifests map[string]ManifestSource
	Pups      map[string]PupStatus
	Internal  *InternalState
	jobs      chan Job
	Changes   chan Change
}

func NewDogeboxd(pupDir string) Dogeboxd {
	intern := InternalState{
		ActionCounter: 100000,
		InstalledPups: []string{"internal.dogeboxd"},
	}
	// TODO: Load state from GOB
	s := Dogeboxd{
		Manifests: map[string]ManifestSource{},
		Pups:      map[string]PupStatus{},
		jobs:      make(chan Job),
		Changes:   make(chan Change),
		Internal:  &intern,
	}
	av := []PupManifest{}
	s.Manifests["local"] = ManifestSource{
		ID:          "local",
		Label:       "Local Filesystem",
		URL:         "",
		LastUpdated: time.Now(),
		Available:   av,
	}

	// Create a synthetic ManifestSource for
	// dogebox itself
	var dbMan PupManifest

	err := json.Unmarshal(dbxManifestFile, &dbMan)
	if err != nil {
		log.Fatalln("Couldn't load Dogeboxd's own manifest")
	}
	dbMan.Hydrate("internal")

	intAv := []PupManifest{dbMan}

	s.loadLocalManifests(pupDir)
	s.Manifests["internal"] = ManifestSource{
		ID:          "internal",
		Label:       "DONT SHOW ME",
		URL:         "",
		LastUpdated: time.Now(),
		Available:   intAv,
	}

	// load pup state from disk
	for _, ip := range s.Internal.InstalledPups {
		fmt.Printf("Loading pup status: %s\n", ip)
		bits := strings.Split(ip, ".")
		fmt.Println("BITS", ip, bits, bits[0], bits[1])
		ms, ok := s.Manifests[bits[0]]
		if !ok {
			fmt.Printf("Failed to load %s, no matching manifest source\n", ip)
			continue
		} else {
			var m PupManifest
			found := false
			for _, man := range ms.Available {
				fmt.Println("a", man.ID, "b", ip)
				if man.ID == ip {
					m = man
					found = true
					break
				}
			}
			if found {
				s.loadPupStatus(pupDir, m)
			} else {
				fmt.Printf("Failed to load %s, no matching manifest\n", ip)
			}
		}
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
					switch a := v.a.(type) {
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
	t.jobs <- Job{a: a, id: id}
	return id
}

func (t Dogeboxd) sendChange(changeType string, j Job) {
	fmt.Println("SENDING CHANGE")
	t.Changes <- Change{ID: j.id, Error: j.err, Type: changeType, Update: j.success}
}

func (t Dogeboxd) GetManifests() map[string]ManifestSource {
	return t.Manifests
}

func (t Dogeboxd) GetPupStats() map[string]PupStatus {
	return t.Pups
}

func (t *Dogeboxd) loadLocalManifests(path string) {
	t.Manifests["local"].UpdateAvailable("local", FindLocalPups(path))
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
		j.err = fmt.Sprintf("Couldnt find pup to update: %s", u.PupID)
		return fmt.Errorf(j.err)
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
		j.err = fmt.Sprintf("Failed to write Pup state to disk %v", err)
		p.Config = old
		return fmt.Errorf(j.err)
	}
	j.success = p
	t.Pups[u.PupID] = p
	return nil
}

type Job struct {
	a       Action
	id      string
	err     string // sent to the client on error via WS
	success any    // will be sent to the client via WS
}

type Change struct {
	ID     string `json:"id"`
	Error  string `json:"error"`
	Type   string `json:"type"`
	Update any    `json:"update"`
}

// InternalState is stored in dogeboxd.gob and contains
// various details about what's installed, what condition
// we're in overall etc.
type InternalState struct {
	ActionCounter int
	InstalledPups []string
}

type SystemJobber interface {
	AddJob(Job)
	GetUpdateChannel() chan Job
}
