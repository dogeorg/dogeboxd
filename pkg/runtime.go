package dogeboxd

type PupStatus struct {
	Stats   map[string][]float32
	Running bool
}

func NewState() State {
	s := State{Manifests: map[string]*ManifestSource{}}
	local := ManifestSource{
		ID:        "local",
		Label:     "Local Filesystem",
		URL:       "",
		Avaialble: []PupManifest{},
		Installed: []PupManifest{},
	}
	s.Manifests["local"] = &local
	return s
}

type State struct {
	Manifests map[string]*ManifestSource
	Pups      *[]PupStatus
}

type ManifestSource struct {
	ID        string
	Label     string
	URL       string
	Avaialble []PupManifest
	Installed []PupManifest
}

func (t State) LoadLocalManifests(path string) {
	manifests := FindLocalPups(path)
	t.Manifests["local"].Avaialble = manifests
}
