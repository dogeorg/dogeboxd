package dogeboxd

import (
	"fmt"
)

/* PupManifest represents a Nix installed process
 * running inside the Dogebox Runtime Environment.
 * These are defined in pup.json files.
 */
type PupManifest struct {
	sourceID         string
	ID               string            `json:"id"`
	Package          string            `json:"package"` // ie:  dogecoin-core
	Hash             string            `json:"hash"`    // package checksum
	Command          CommandManifest   `json:"command"`
	PermissionGroups []PermissionGroup `json:"permissionGroups"`
	Dependencies     []Dependency      `json:"dependencies"`
	hydrated         bool
	containerIP      string
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

/* PermissionGroups define how other
 * pups can request access to this pup's
 * APIs and resources, via their Dependencies
 */
type PermissionGroup struct {
	Name        string   `json:"name"`        // ie:  admin, wallet-read-only, etc.
	Description string   `json:"description"` // What does this permission group do (shown to user)
	Severity    int      `json:"severity"`    // 1-3, 1: critical/danger, 2: makes changes, 3: read only stuff
	Routes      []string `json:"routes"`      // http routes accessible for this group
}

/* Dependency specifies that this pup requires
 * another pup to be running, and what permission
 * groups from that pup need to be available.
 */
type Dependency struct {
	PupID            string   `json:"pupID"`            // pup that we depend on
	PermissionGroups []string `json:"permissionGroups"` // list of permission groups from that pup we want access to
	// Version          string   `json:"version"`          // min version of the pup required
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
