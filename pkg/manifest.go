package dogeboxd

/* PupManifest represents a Nix installed process
 * running inside the Dogebox Runtime Environment.
 * These are defined in pup.json files.
 */
type PupManifest struct {
	Package string          `json:"package"` // ie:  dogebox.dogecoin-core
	Hash    string          `json:"hash"`    // package checksum
	Command CommandManifest `json:"command"`
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
	ID        string        `json:"id"`
	Label     string        `json:"label"`
	URL       string        `json:"url"`
	Available []PupManifest `json:"available"`
	Installed []PupManifest `json:"installed"`
}

func (t State) LoadLocalManifests(path string) {
	manifests := FindLocalPups(path)
	t.Manifests["local"].Available = manifests
}
