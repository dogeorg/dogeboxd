package dogeboxd

import (
	"fmt"
	"strings"

	"github.com/dogeorg/dogeboxd/pkg/pup"
)

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

func (t ManifestIndex) FindManifest(manID string) (pup.PupManifest, bool) {
	sourceID, _, ok := strings.Cut(manID, ".")
	if !ok {
		return pup.PupManifest{}, false
	}
	source, ok := t.GetSource(sourceID)
	if !ok {
		return pup.PupManifest{}, false
	}
	return source.FindManifest(manID)
}
