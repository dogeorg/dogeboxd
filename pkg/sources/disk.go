package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

var _ dogeboxd.ManifestSource = &ManifestSourceDisk{}

type ManifestSourceDisk struct {
	config dogeboxd.ManifestSourceConfiguration
}

func (r ManifestSourceDisk) ConfigFromLocation(location string) (dogeboxd.ManifestSourceConfiguration, error) {
	dogeboxPath := filepath.Join(location, "dogebox.json")
	if _, err := os.Stat(dogeboxPath); err == nil {
		content, err := os.ReadFile(dogeboxPath)
		if err != nil {
			return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("failed to read dogebox.json: %w", err)
		}

		details, err := ParseAndValidateSourceDetails(string(content))
		if err != nil {
			return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("failed to parse and validate dogebox.json: %w", err)
		}

		return dogeboxd.ManifestSourceConfiguration{
			ID:          details.ID,
			Name:        details.Name,
			Description: details.Description,
			Location:    location,
			Type:        "disk",
		}, nil
	} else if os.IsNotExist(err) {
		folder := filepath.Base(location)

		return dogeboxd.ManifestSourceConfiguration{
			ID:          folder,
			Name:        folder,
			Description: "",
			Location:    location,
			Type:        "disk",
		}, nil
	} else {
		return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("error accessing dogebox.json: %w", err)
	}
}

func (r ManifestSourceDisk) Config() dogeboxd.ManifestSourceConfiguration {
	return r.config
}

func (r ManifestSourceDisk) Validate() (bool, error) {
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

	dogeboxPath := filepath.Join(r.config.Location, "dogebox.json")
	if _, err := os.Stat(dogeboxPath); err == nil {
		content, err := os.ReadFile(dogeboxPath)
		if err != nil {
			return false, fmt.Errorf("failed to read dogebox.json: %w", err)
		}

		_, err = ParseAndValidateSourceDetails(string(content))
		if err != nil {
			return false, fmt.Errorf("failed to parse and validate dogebox.json: %w", err)
		}

		// If we have a root dogebox.json, this is a valid repo (even if there are no pups)
		return true, nil
	} else if os.IsNotExist(err) {
		valid, err := r.validatePup(r.config.Location)

		if valid {
			return true, nil
		}

		if !valid || err != nil {
			return false, fmt.Errorf("failed to validate pup at root location %s: %w", r.config.Location, err)
		}
	} else {
		return false, fmt.Errorf("error accessing dogebox.json: %w", err)
	}

	return false, fmt.Errorf("failed to validate source at %s", r.config.Location)
}

func (r ManifestSourceDisk) validatePup(location string) (bool, error) {
	for _, filename := range REQUIRED_FILES {
		// TODO: probably validate these are well-structured.
		p := filepath.Join(location, filename)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return false, fmt.Errorf("%s not found in %s", filename, r.config.Location)
		}
	}

	return true, nil
}

func (r ManifestSourceDisk) List(_ bool) (dogeboxd.ManifestSourceList, error) {
	dogeboxPath := filepath.Join(r.config.Location, "dogebox.json")

	pupLocations := []string{}

	if _, err := os.Stat(dogeboxPath); err == nil {
		content, err := os.ReadFile(dogeboxPath)
		if err != nil {
			return dogeboxd.ManifestSourceList{}, fmt.Errorf("failed to read dogebox.json: %w", err)
		}

		sourceIndex, err := ParseAndValidateSourceDetails(string(content))
		if err != nil {
			return dogeboxd.ManifestSourceList{}, fmt.Errorf("failed to parse and validate dogebox.json: %w", err)
		}

		for _, pupDetail := range sourceIndex.Pups {
			pupLocations = append(pupLocations, filepath.Join(r.config.Location, pupDetail.Location))
		}
	} else {
		pupLocations = append(pupLocations, r.config.Location)
	}

	pups := []dogeboxd.ManifestSourcePup{}

	for _, pupLocation := range pupLocations {
		manifestPath := filepath.Join(pupLocation, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			return dogeboxd.ManifestSourceList{}, fmt.Errorf("failed to read manifest file: %w", err)
		}

		var manifest pup.PupManifest
		err = json.Unmarshal(manifestData, &manifest)
		if err != nil {
			return dogeboxd.ManifestSourceList{}, fmt.Errorf("failed to parse manifest file: %w", err)
		}

		if err := manifest.Validate(); err != nil {
			return dogeboxd.ManifestSourceList{}, fmt.Errorf("manifest validation failed: %w", err)
		}

		pup := dogeboxd.ManifestSourcePup{
			Name:     manifest.Meta.Name,
			Location: r.config.Location,
			Version:  manifest.Meta.Version,
			Manifest: manifest,
		}

		pups = append(pups, pup)
	}

	return dogeboxd.ManifestSourceList{
		LastUpdated: time.Now(),
		Pups:        pups,
	}, nil
}

func (r ManifestSourceDisk) Download(diskPath string, remoteLocation string) error {
	return nil
}
