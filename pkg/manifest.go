package dogeboxd

import (
	"fmt"
	"time"
)

/* PupManifest represents a Nix installed process
 * running inside the Dogebox Runtime Environment.
 * These are defined in pup.json files.
 */
type PupManifest struct {
	sourceID string
	ID       string          `json:"id"`
	Package  string          `json:"package"` // ie:  dogecoin-core
	Hash     string          `json:"hash"`    // package checksum
	Command  CommandManifest `json:"command"`
}

// This is called when a Pup is loaded from storage, JSON/GOB etc.
func (t *PupManifest) Hydrate(sourceID string) {
	t.sourceID = sourceID
	t.ID = fmt.Sprintf("%s.%s", sourceID, t.Package)
	fmt.Println("HYDRATED: ", t.ID)
}

/* Represents the command to run inside this PUP
 * Container.
 */
type CommandManifest struct {
	Path        string            `json:"path"`
	Args        string            `json:"args"`
	CWD         string            `json:"cwd"`
	ENV         map[string]string `json:"env"`
	Config      ConfigFields      `json:"config"`
	ConfigFiles []ConfigFile      `json:"configFiles"`
}

/* Represents a Config file that needs to be written
 * inside the DRE filesystem at Path, Template is a
 * text/template string which will be filled with
 * values provided by CommandManifest.Config.
 */
type ConfigFile struct {
	Template string
	Path     string
}

/* Represents fields that are user settable, which provide the values
 * for templates (Args, ENV, ConfigFiles), we only care about Name
 */
type ConfigFields struct {
	Sections []struct {
		Name   string                   `json:"name"`
		Label  string                   `json:"label"`
		Fields []map[string]interface{} `json:"fields"`
	} `json:"sections"`
}

// A ManifestSource is a .. well, it's a source of manifests, derp.
type ManifestSource struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	URL         string        `json:"url"`
	LastUpdated time.Time     `json:"lastUpdated"`
	Available   []PupManifest `json:"available"`
}

// Append or replace available pups
func (t ManifestSource) UpdateAvailable(sourceID string, l []PupManifest) {
	exists := map[string]int{}
	for i, p := range t.Available {
		exists[p.ID] = i
	}

	for _, p := range l {
		p.Hydrate(sourceID)
		fmt.Printf("==== hydrated %s\n", p.ID)
		i, ok := exists[p.ID]
		if ok {
			t.Available = append((t.Available)[:i], (t.Available)[i+1:]...)
		}
		t.Available = append(t.Available, p)
	}
}
