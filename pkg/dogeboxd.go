package dogeboxd

func LoadState() State {
	s := State{Manifests: map[string]*ManifestSource{}}
	local := ManifestSource{
		ID:        "local",
		Label:     "Local Filesystem",
		URL:       "",
		Available: []PupManifest{},
		Installed: []PupManifest{},
	}
	s.Manifests["local"] = &local
	return s
}

// State for the running dogeboxd comes from a number
// of places, Manifests fetched from the internet, local on-disk
// 'dev' pups, pup status is loaded from pup.gob files and overall
// state from dogeboxd's own internal.gob file.
type State struct {
	Manifests map[string]*ManifestSource
	Pups      map[string]*PupStatus
	Internal  *InternalState
}

// create or load PupStatus for a given PUP id
func (t State) LoadPupStatus(id string, config ServerConfig) {
	p := PupStatus{ID: id}
	p.Read(config.PupDir)
	t.Pups[id] = &p
}

// InternalState is stored in dogeboxd.gob and contains
// various details about what's installed, what condition
// we're in overall etc.
type InternalState struct{}
