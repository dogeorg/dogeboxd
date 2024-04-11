package dogeboxd

import (
	"context"
	"fmt"
	"time"
)

type Dogeboxd struct {
	Manifests map[string]ManifestSource
	Pups      map[string]PupStatus
	// Internal  *InternalState
	jobs chan job
	// Changes   chan<- Change
}

func NewDogeboxd(pupDir string) Dogeboxd {
	s := Dogeboxd{
		Manifests: map[string]ManifestSource{},
		Pups:      map[string]PupStatus{},
		jobs:      make(chan job),
	}
	av := []PupManifest{}
	s.Manifests["local"] = ManifestSource{
		ID:          "local",
		Label:       "Local Filesystem",
		URL:         "",
		LastUpdated: time.Now(),
		Available:   &av,
	}
	s.loadLocalManifests(pupDir)
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
	id := "asdf"
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
	p.Read(config.PupDir)
	t.Pups[id] = p
}

type job struct {
	a  Action
	id string
}

// InternalState is stored in dogeboxd.gob and contains
// various details about what's installed, what condition
// we're in overall etc.
type InternalState struct{}
