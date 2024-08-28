package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

var _ dogeboxd.ManifestRepository = &ManifestRepositoryDisk{}

type ManifestRepositoryDisk struct {
	config dogeboxd.ManifestRepositoryConfiguration
}

func (r ManifestRepositoryDisk) Name() string {
	return r.config.Name
}

func (r ManifestRepositoryDisk) Config() dogeboxd.ManifestRepositoryConfiguration {
	return r.config
}

func (r ManifestRepositoryDisk) Validate() (bool, error) {
	// Check if the folder exists
	folderInfo, err := os.Stat(r.config.Location)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("folder %s does not exist", r.config.Location)
		}
		return false, fmt.Errorf("error accessing folder %s: %w", r.config.Location, err)
	}

	if !folderInfo.IsDir() {
		return false, fmt.Errorf("%s is not a directory", r.config.Location)
	}

	for _, filename := range REQUIRED_FILES {
		// TODO: probably validate these are well-structured.
		p := filepath.Join(r.config.Location, filename)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return false, fmt.Errorf("%s not found in %s", filename, r.config.Location)
		}
	}

	return true, nil
}

func (r ManifestRepositoryDisk) List(_ bool) (dogeboxd.ManifestRepositoryList, error) {
	// At the moment we only support a single pup per repository.
	// This will change in the future with the introduction of a root
	// dogebox.json or something that can point to sub-pups.

	// Load the manifest file
	manifestPath := filepath.Join(r.config.Location, "pup.manifest")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return dogeboxd.ManifestRepositoryList{}, fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest pup.PupManifest
	err = json.Unmarshal(manifestData, &manifest)
	if err != nil {
		return dogeboxd.ManifestRepositoryList{}, fmt.Errorf("failed to parse manifest file: %w", err)
	}

	pup := dogeboxd.ManifestRepositoryPup{
		Name:     r.config.Name,
		Location: r.config.Location,
		Version:  manifest.Meta.Version,
		Manifest: manifest,
	}

	return dogeboxd.ManifestRepositoryList{
		LastUpdated: time.Now(),
		Pups:        []dogeboxd.ManifestRepositoryPup{pup},
	}, nil
}

func (r ManifestRepositoryDisk) Download(diskPath string, remoteLocation string) error {
	// At the moment we only support a single pup per repository,
	// so we can ignore the name field here, eventually it will be used.
	// For a disk repository, we always just return what is on-disk, unversioned.
	return nil
}
