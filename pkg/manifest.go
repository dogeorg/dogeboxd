package dogeboxd

import (
	"fmt"
	"strings"
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
	hydrated bool
}

// This is called when a Pup is loaded from storage, JSON/GOB etc.
func (t *PupManifest) Hydrate(sourceID string) {
	if t.hydrated {
		return
	}
	t.sourceID = sourceID
	t.ID = fmt.Sprintf("%s.%s", sourceID, t.Package)
	t.hydrated = true
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

/* A ManifestSource represents an origin of PUP manifests, usually
 * a webserver and NIX repository. These are accessed via the
 * ManifestIndex (below)
 */
type ManifestSource interface {
	FindManifestByPupID(string) (PupManifest, bool)
	Export() ManifestSourceExport
}

// returned by ManifestSource.Export, represents the ManifestSource
// to the browser.
type ManifestSourceExport struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	URL         string        `json:"url"`
	LastUpdated time.Time     `json:"lastUpdated"`
	Available   []PupManifest `json:"available"`
}

func (t ManifestSourceExport) Export() ManifestSourceExport {
	return t
}

/* The ManifestIndex is collection of ManifestSources with methods for
 * lookup across all sources etc.
 */

type ManifestIndex struct {
	sources map[string]ManifestSource
}

func NewManifestIndex() ManifestIndex {
	return ManifestIndex{
		sources: map[string]ManifestSource{},
	}
}

func (t ManifestIndex) AddSource(name string, m ManifestSource) error {
	_, exists := t.sources[name]
	if exists {
		return fmt.Errorf("Source already added %s", name)
	}
	t.sources[name] = m
	return nil
}

func (t ManifestIndex) GetManifestMap() map[string]ManifestSourceExport {
	o := map[string]ManifestSourceExport{}
	for k, v := range t.sources {
		o[k] = v.Export()
	}
	return o
}

func (t ManifestIndex) GetSource(name string) (ManifestSource, bool) {
	s, ok := t.sources[name]
	if !ok {
		return nil, false
	}
	return s, true
}

func (t ManifestIndex) FindManifest(pupID string) (PupManifest, bool) {
	sourceID, _, ok := strings.Cut(pupID, ".")
	if !ok {
		return PupManifest{}, false
	}
	source, ok := t.GetSource(sourceID)
	if !ok {
		return PupManifest{}, false
	}
	return source.FindManifestByPupID(pupID)
}
