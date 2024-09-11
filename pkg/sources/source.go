package source

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

var REQUIRED_FILES = []string{"pup.nix", "manifest.json"}

func NewSourceManager(config dogeboxd.ServerConfig, sm dogeboxd.StateManager, pm dogeboxd.PupManager) dogeboxd.SourceManager {
	state := sm.Get().Sources

	sources := []dogeboxd.ManifestSource{}
	for _, c := range state.SourceConfigs {
		switch c.Type {
		case "disk":
			sources = append(sources, ManifestSourceDisk{config: c})
		case "git":
			sources = append(sources, &ManifestSourceGit{serverConfig: config, config: c})
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

func (sourceManager *sourceManager) GetAll(ignoreCache bool) (map[string]dogeboxd.ManifestSourceList, error) {
	available := map[string]dogeboxd.ManifestSourceList{}

	for _, r := range sourceManager.sources {
		l, err := r.List(ignoreCache)
		if err != nil {
			return nil, err
		}

		available[l.Config.ID] = l
	}

	return available, nil
}

func (sourceManager *sourceManager) GetSourceManifest(sourceID, pupName, pupVersion string) (pup.PupManifest, dogeboxd.ManifestSource, error) {
	for _, r := range sourceManager.sources {
		c := r.Config()
		if c.ID == sourceID {
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

	return pup.PupManifest{}, nil, fmt.Errorf("no source found with id %s", sourceID)
}

func (sourceManager *sourceManager) GetSourcePup(sourceId, pupName, pupVersion string) (dogeboxd.ManifestSourcePup, error) {
	r, err := sourceManager.GetSource(sourceId)
	if err != nil {
		return dogeboxd.ManifestSourcePup{}, err
	}

	l, err := r.List(false)
	if err != nil {
		return dogeboxd.ManifestSourcePup{}, err
	}

	for _, pup := range l.Pups {
		if pup.Name == pupName && pup.Version == pupVersion {
			log.Printf("getSourcePup Location: %+v", pup.Location)
			return pup, nil
		}
	}

	return dogeboxd.ManifestSourcePup{}, fmt.Errorf("no pup found with name %s and version %s", pupName, pupVersion)
}

func (sourceManager *sourceManager) GetSource(id string) (dogeboxd.ManifestSource, error) {
	for _, r := range sourceManager.sources {
		c := r.Config()
		if c.ID == id {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no source found with id %s", id)
}

func (sourceManager *sourceManager) DownloadPup(path, sourceId, pupName, pupVersion string) error {
	r, err := sourceManager.GetSource(sourceId)
	if err != nil {
		return err
	}

	sourcePup, err := sourceManager.GetSourcePup(sourceId, pupName, pupVersion)
	if err != nil {
		return err
	}

	log.Printf("got source pup: %+v", sourcePup)

	if err := r.Download(path, sourcePup.Location); err != nil {
		return err
	}

	manifestPath := filepath.Join(path, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest pup.PupManifest
	err = json.Unmarshal(manifestData, &manifest)
	if err != nil {
		return fmt.Errorf("failed to parse manifest file: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("manifest validation failed: %w", err)
	}

	return sourceManager.validatePupFiles(path)
}

func (sourceManager *sourceManager) validatePupFiles(path string) error {
	manifestPath := filepath.Join(path, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest pup.PupManifest
	err = json.Unmarshal(manifestData, &manifest)
	if err != nil {
		return fmt.Errorf("failed to parse manifest file: %w", err)
	}

	nixFilePath := filepath.Join(path, manifest.Container.Build.NixFile)
	if _, err := os.Stat(nixFilePath); os.IsNotExist(err) {
		return fmt.Errorf("nix file %s not found", manifest.Container.Build.NixFile)
	}

	if manifest.Meta.LogoPath != "" {
		logoFilePath := filepath.Join(path, manifest.Meta.LogoPath)
		if _, err := os.Stat(logoFilePath); os.IsNotExist(err) {
			return fmt.Errorf("logo file %s not found", manifest.Meta.LogoPath)
		}
	}

	return nil
}

func (sourceManager *sourceManager) GetAllSourceConfigurations() []dogeboxd.ManifestSourceConfiguration {
	configs := []dogeboxd.ManifestSourceConfiguration{}
	for _, s := range sourceManager.sources {
		configs = append(configs, s.Config())
	}
	return configs
}

func (sourceManager *sourceManager) determineSourceType(location string) (string, error) {
	if strings.HasPrefix(location, "https://") && strings.HasSuffix(location, ".git") {
		return "git", nil
	}

	if strings.HasPrefix(location, "git@") {
		return "git", nil
	}

	if strings.HasPrefix(location, "/") {
		if _, err := os.Stat(location); err != nil {
			return "", fmt.Errorf("location looks like disk path, but path %s does not exist", location)
		}

		return "disk", nil
	}

	return "", fmt.Errorf("unknown source type: %s", location)
}

func (sourceManager *sourceManager) AddSource(location string) (dogeboxd.ManifestSource, error) {
	var c dogeboxd.ManifestSourceConfiguration
	var s dogeboxd.ManifestSource

	sourceType, err := sourceManager.determineSourceType(location)
	if err != nil || sourceType == "" {
		return nil, err
	}

	switch sourceType {
	case "disk":
		{
			config, err := ManifestSourceDisk{}.ValidateFromLocation(location)
			if err != nil {
				return nil, err
			}
			c = config
			s = &ManifestSourceDisk{config: config}
		}
	case "git":
		{
			config, err := ManifestSourceGit{}.ValidateFromLocation(location)
			if err != nil {
				return nil, err
			}
			c = config
			s = &ManifestSourceGit{config: config}
		}

	default:
		return nil, fmt.Errorf("unknown source type: %s", sourceType)
	}

	log.Printf("generated config: %+v", c)

	// Ensure no existing source has the same id
	for _, _s := range sourceManager.sources {
		_c := _s.Config()
		if _c.ID == c.ID {
			log.Printf("source with id %s already exists", c.ID)
			return nil, fmt.Errorf("source with id %s already exists", c.ID)
		}
		if _c.Location == c.Location {
			log.Printf("source with location %s already exists", c.Location)
			return nil, fmt.Errorf("source with location %s already exists", c.Location)
		}
	}

	sourceManager.sources = append(sourceManager.sources, s)
	if err := sourceManager.Save(); err != nil {
		log.Println("error while saving sources:", err)
		return nil, err
	}

	return s, nil
}

func (sourceManager *sourceManager) RemoveSource(id string) error {
	var matched dogeboxd.ManifestSource
	var matchedIndex int

	for i, r := range sourceManager.sources {
		c := r.Config()
		if c.ID == id {
			matched = r
			matchedIndex = i
		}
	}

	if matched == nil {
		return fmt.Errorf("no existing source id: %s", id)
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
