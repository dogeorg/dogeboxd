package source

import (
	"errors"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

var REQUIRED_FILES = []string{"pup.nix", "manifest.json"}

func NewSourceManager(sm dogeboxd.StateManager, pm dogeboxd.PupManager) dogeboxd.SourceManager {
	state := sm.Get().Sources

	return sourceManager{
		sm:      sm,
		pm:      pm,
		sources: state.Sources,
	}
}

var _ dogeboxd.SourceManager = &sourceManager{}

type sourceManager struct {
	sm      dogeboxd.StateManager
	pm      dogeboxd.PupManager
	sources []dogeboxd.ManifestSource
}

func (sourceManager sourceManager) GetAll() (map[string]dogeboxd.ManifestSourceList, error) {
	available := map[string]dogeboxd.ManifestSourceList{}

	for _, r := range sourceManager.sources {
		l, err := r.List(false)
		if err != nil {
			return nil, err
		}

		available[r.Name()] = l
	}

	return available, nil
}

func (sourceManager sourceManager) GetSourceManifest(sourceName, pupName, pupVersion string) (pup.PupManifest, error) {
	for _, r := range sourceManager.sources {
		if r.Name() == sourceName {
			list, err := r.List(false)
			if err != nil {
				return pup.PupManifest{}, err
			}
			for _, pup := range list.Pups {
				if pup.Name == pupName && pup.Version == pupVersion {
					return pup.Manifest, nil
				}
			}
			return pup.PupManifest{}, fmt.Errorf("no pup found with name %s and version %s", pupName, pupVersion)
		}
	}

	return pup.PupManifest{}, fmt.Errorf("no source found with name %s", sourceName)
}

func (sourceManager sourceManager) GetSourcePup(sourceName, pupName, pupVersion string) (dogeboxd.ManifestSourcePup, error) {
	r, err := sourceManager.GetSource(sourceName)
	if err != nil {
		return dogeboxd.ManifestSourcePup{}, err
	}

	l, err := r.List(false)
	if err != nil {
		return dogeboxd.ManifestSourcePup{}, err
	}

	for _, pup := range l.Pups {
		if pup.Name == pupName && pup.Version == pupVersion {
			return pup, nil
		}
	}

	return dogeboxd.ManifestSourcePup{}, fmt.Errorf("no pup found with name %s and version %s", pupName, pupVersion)
}

func (sourceManager sourceManager) GetSource(name string) (dogeboxd.ManifestSource, error) {
	for _, r := range sourceManager.sources {
		if r.Name() == name {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no source found with name %s", name)
}

func (sourceManager sourceManager) DownloadPup(path, sourceName, pupName, pupVersion string) error {
	r, err := sourceManager.GetSource(sourceName)
	if err != nil {
		return err
	}

	sourcePup, err := sourceManager.GetSourcePup(sourceName, pupName, pupVersion)

	return r.Download(path, sourcePup.Location)
}

func (sourceManager sourceManager) AddSource(source dogeboxd.ManifestSourceConfiguration) (dogeboxd.ManifestSource, error) {
	// Ensure no existing source has the same name
	for _, _s := range sourceManager.sources {
		if _s.Name() == source.Name {
			return nil, fmt.Errorf("source with name %s already exists", source.Name)
		}
	}

	var s dogeboxd.ManifestSource

	switch source.Type {
	case "local":
		s = ManifestSourceDisk{config: source}
	case "git":
		s = ManifestSourceGit{config: source}

	default:
		return nil, fmt.Errorf("unknown source type: %s", source.Type)
	}

	valid, err := s.Validate()

	if err != nil {
		log.Println("error while validating source:", err)
		return nil, err
	}

	if !valid {
		return nil, errors.New("source failed to validate")
	}

	sourceManager.sources = append(sourceManager.sources, s)

	state := sourceManager.sm.Get().Sources
	state.Sources = sourceManager.sources
	sourceManager.sm.SetSources(state)
	if err := sourceManager.sm.Save(); err != nil {
		return nil, err
	}

	return s, nil
}

func (sourceManager sourceManager) RemoveSource(name string) error {
	var matched dogeboxd.ManifestSource
	var matchedIndex int

	for i, r := range sourceManager.sources {
		if r.Name() == name {
			matched = r
			matchedIndex = i
		}
	}

	if matched == nil {
		return fmt.Errorf("no existing source named %s", name)
	}

	// Check if we have an existing pup that is from
	// this source if we do, we don't let removal happen.
	installedPups := sourceManager.pm.GetAllFromSource(matched.Config())

	if len(installedPups) != 0 {
		return fmt.Errorf("%d installed pups using this source, aborting", len(installedPups))
	}

	sourceManager.sources = append(sourceManager.sources[:matchedIndex], sourceManager.sources[matchedIndex+1:]...)

	state := sourceManager.sm.Get().Sources
	state.Sources = sourceManager.sources
	sourceManager.sm.SetSources(state)
	if err := sourceManager.sm.Save(); err != nil {
		return err
	}

	return nil
}
