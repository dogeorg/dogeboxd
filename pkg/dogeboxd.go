package dogeboxd

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

//go:embed pup.json
var dbxManifestFile []byte

type Dogeboxd struct {
	Manifests map[string]ManifestSource
	Pups      map[string]PupStatus
	Internal  *InternalState
	jobs      chan job
	// Changes   chan<- Change
}

func NewDogeboxd(pupDir string) Dogeboxd {
	intern := InternalState{
		ActionCounter: 100000,
	}
	// TODO: Load state from GOB
	s := Dogeboxd{
		Manifests: map[string]ManifestSource{},
		Pups:      map[string]PupStatus{},
		jobs:      make(chan job),
		Internal:  &intern,
	}
	av := []PupManifest{}
	s.Manifests["local"] = ManifestSource{
		ID:          "local",
		Label:       "Local Filesystem",
		URL:         "",
		LastUpdated: time.Now(),
		Available:   &av,
	}

	// Create a synthetic ManifestSource for
	// dogebox itself
	var dbMan PupManifest

	err := json.Unmarshal(dbxManifestFile, &dbMan)
	if err != nil {
		log.Fatalln("Couldn't load Dogeboxd's own manifest")
	}

	intAv := []PupManifest{dbMan}

	s.loadLocalManifests(pupDir)
	s.Manifests["internal"] = ManifestSource{
		ID:          "internal",
		Label:       "DONT SHOW ME",
		URL:         "",
		LastUpdated: time.Now(),
		Available:   &intAv,
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
					switch v := v.a.(type) {
					case LoadLocalPup:
						fmt.Println("Load local pup from ", v.Path)
					case UpdatePupConfig:
						fmt.Printf("Update pup config %v\n", v)
						t.updatePupConfig(v)
					default:
						fmt.Printf("Unknown action type: %v\n", v)
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
	t.jobs <- job{a, id}
	return id
}

func (t Dogeboxd) GetManifests() map[string]ManifestSource {
	return t.Manifests
}

func (t Dogeboxd) GetPupStats() map[string]PupStatus {
	return t.Pups
}

func (t Dogeboxd) loadLocalManifests(path string) {
	t.Manifests["local"].UpdateAvailable(FindLocalPups(path))
}

// create or load PupStatus for a given PUP id
func (t Dogeboxd) loadPupStatus(id string, config ServerConfig) {
	p := PupStatus{ID: id}
	p.Read()
	t.Pups[id] = p
}

func (t Dogeboxd) updatePupConfig(u UpdatePupConfig) error {
	p, ok := t.Pups[u.PupID]
	if !ok {
		fmt.Printf("Couldnt find pup to update: %s", u.PupID)
	}
	fmt.Println(p)
	return nil
}

type job struct {
	a  Action
	id string
}

// InternalState is stored in dogeboxd.gob and contains
// various details about what's installed, what condition
// we're in overall etc.
type InternalState struct {
	ActionCounter int
}
