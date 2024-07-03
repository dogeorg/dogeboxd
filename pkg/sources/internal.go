package sources

import (
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type InternalSource struct {
	dogeboxd.ManifestSourceExport
}

func NewInternalSource() InternalSource {
	s := InternalSource{}
	s.ID = "internal"
	s.Label = "Internal"
	s.URL = ""
	s.LastUpdated = time.Now()
	s.Available = []dogeboxd.PupManifest{}
	return s
}

func (t InternalSource) FindManifestByPupID(id string) (dogeboxd.PupManifest, bool) {
	for _, m := range t.Available {
		if m.ID == id {
			return m, true
		}
	}
	return dogeboxd.PupManifest{}, false
}

func (t InternalSource) AddManifest(m dogeboxd.PupManifest) {
	m.Hydrate("internal")
	t.Available = append(t.Available, m)
}
