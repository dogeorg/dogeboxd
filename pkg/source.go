package dogeboxd

import (
	"time"
)

/* A ManifestSource represents an origin of PUP manifests, usually
 * a webserver and NIX repository. These are accessed via the
 * ManifestIndex, see also pkg/sources
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
