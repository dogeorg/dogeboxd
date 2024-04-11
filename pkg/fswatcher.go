package dogeboxd

import (
	"context"
	"log"

	"github.com/fsnotify/fsnotify"
)

// Watcher Service monitors important files and updates State as needed
type Watcher struct {
	paths   []string
	watcher fsnotify.Watcher
}

func NewWatcher(pupDir string) Watcher {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	w := Watcher{
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

/* in manifest.go?
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
*/
