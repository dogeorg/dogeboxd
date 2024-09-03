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

	sources := []dogeboxd.ManifestSource{}
	for _, c := range state.SourceConfigs {
		switch c.Type {
		case "local":
			sources = append(sources, ManifestSourceDisk{config: c})
		case "git":
			sources = append(sources, &ManifestSourceGit{config: c})
		}
	}

	log.Printf("Loaded %d sources", len(sources))

	sourceManager := sourceManager{
		sm:      sm,
		pm:      pm,
		sources: sources,
	}

	return &sourceManager
}

var _ dogeboxd.SourceManager = &sourceManager{}

type sourceManager struct {
	sm      dogeboxd.StateManager
	pm      dogeboxd.PupManager
	sources []dogeboxd.ManifestSource
}

func (sourceManager *sourceManager) GetAll() (map[string]dogeboxd.ManifestSourceList, error) {
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

func (sourceManager *sourceManager) GetSourceManifest(sourceName, pupName, pupVersion string) (pup.PupManifest, dogeboxd.ManifestSource, error) {
	for _, r := range sourceManager.sources {
		if r.Name() == sourceName {
			list, err := r.List(false)
			if err != nil {
				return pup.PupManifest{}, nil, err
			}
			for _, pup := range list.Pups {
				if pup.Name == pupName && pup.Version == pupVersion {
					return pup.Manifest, r, nil
				}
			}
			return pup.PupManifest{}, nil, fmt.Errorf("no pup found with name %s and version %s", pupName, pupVersion)
		}
	}

	return pup.PupManifest{}, nil, fmt.Errorf("no source found with name %s", sourceName)
}

func (sourceManager *sourceManager) GetSourcePup(sourceName, pupName, pupVersion string) (dogeboxd.ManifestSourcePup, error) {
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

func (sourceManager *sourceManager) GetSource(name string) (dogeboxd.ManifestSource, error) {
	for _, r := range sourceManager.sources {
		if r.Name() == name {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no source found with name %s", name)
}

func (sourceManager *sourceManager) DownloadPup(path, sourceName, pupName, pupVersion string) error {
	r, err := sourceManager.GetSource(sourceName)
	if err != nil {
		return err
	}

	sourcePup, err := sourceManager.GetSourcePup(sourceName, pupName, pupVersion)
	if err != nil {
		return err
	}

	return r.Download(path, sourcePup.Location)
}

func (sourceManager *sourceManager) GetAllSourceConfigurations() []dogeboxd.ManifestSourceConfiguration {
	configs := []dogeboxd.ManifestSourceConfiguration{}
	for _, s := range sourceManager.sources {
		configs = append(configs, s.Config())
	}
	return configs
}

func (sourceManager *sourceManager) AddSource(source dogeboxd.ManifestSourceConfiguration) (dogeboxd.ManifestSource, error) {
	// Ensure no existing source has the same name
	for _, _s := range sourceManager.sources {
		if _s.Name() == source.Name {
			log.Printf("source with name %s already exists", source.Name)
			return nil, fmt.Errorf("source with name %s already exists", source.Name)
		}
	}

	var s dogeboxd.ManifestSource

	switch source.Type {
	case "local":
		s = ManifestSourceDisk{config: source}
	case "git":
		s = &ManifestSourceGit{config: source}

	default:
		log.Printf("unknown source type: %s", source.Type)
		return nil, fmt.Errorf("unknown source type: %s", source.Type)
	}

	valid, err := s.Validate()

	if err != nil {
		log.Println("error while validating source:", err)
		return nil, err
	}

	if !valid {
		log.Println("source failed to validate")
		return nil, errors.New("source failed to validate")
	}

	sourceManager.sources = append(sourceManager.sources, s)
	if err := sourceManager.Save(); err != nil {
		log.Println("error while saving sources:", err)
		return nil, err
	}

	return s, nil
}

func (sourceManager *sourceManager) RemoveSource(name string) error {
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
	if err := sourceManager.Save(); err != nil {
		return err
	}

	return nil
}

func (sourceManager *sourceManager) Save() error {
	state := sourceManager.sm.Get().Sources
	state.SourceConfigs = sourceManager.GetAllSourceConfigurations()
	sourceManager.sm.SetSources(state)
	return sourceManager.sm.Save()
}
