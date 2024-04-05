package dogeboxd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/fsnotify/fsnotify"
)

// State for the running dogeboxd comes from a number
// of places, Manifests fetched from the internet, local on-disk
// 'dev' pups, pup status is loaded from pup.gob files and overall
// state from dogeboxd's own internal.gob file.
type State struct {
	Manifests map[string]ManifestSource
	Pups      map[string]PupStatus
	Internal  *InternalState
}

func LoadState(pupDir string) State {
	s := State{Manifests: map[string]ManifestSource{}}
	av := []PupManifest{}
	ins := []PupManifest{}
	s.Manifests["local"] = ManifestSource{
		ID:        "local",
		Label:     "Local Filesystem",
		URL:       "",
		Available: &av,
		Installed: &ins,
	}
	s.loadLocalManifests(pupDir)
	return s
}

func (t State) loadLocalManifests(path string) {
	t.Manifests["local"].UpdateAvailable(FindLocalPups(path))
}

// create or load PupStatus for a given PUP id
func (t State) loadPupStatus(id string, config ServerConfig) {
	p := PupStatus{ID: id}
	p.Read(config.PupDir)
	t.Pups[id] = p
}

// InternalState is stored in dogeboxd.gob and contains
// various details about what's installed, what condition
// we're in overall etc.
type InternalState struct{}

// Watcher Service monitors important files and updates State as needed
type Watcher struct {
	paths   []string
	state   State
	watcher fsnotify.Watcher
}

func NewWatcher(state State, pupDir string) Watcher {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	w := Watcher{
		state:   state,
		watcher: *fsw,
	}
	err = fsw.Add(pupDir)
	if err != nil {
		log.Fatal(err)
	}
	return w
}

func (t Watcher) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
			for {
				select {
				case event, ok := <-t.watcher.Events:
					if !ok {
						return
					}
					log.Println("watcher event:", event)
					if event.Has(fsnotify.Write) {
						log.Println("watcher modified file:", event.Name)
					}
				case err, ok := <-t.watcher.Errors:
					if !ok {
						return
					}
					log.Println("watcher error:", err)
				}
			}
		}()
		started <- true
		<-stop
		t.watcher.Close()
		stopped <- true
	}()
	return nil
}

func isPupDir(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		fmt.Println("watcher failed to stat", path)
		return false
	}
	if fileInfo.IsDir() {
	} else {
		return false
	}
}
