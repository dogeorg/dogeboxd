package source

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
	"github.com/dogeorg/dogeboxd/pkg/utils"
)

var _ dogeboxd.ManifestSource = &ManifestSourceDisk{}

type ManifestSourceDisk struct {
	config dogeboxd.ManifestSourceConfiguration
}

func (r ManifestSourceDisk) ValidateFromLocation(location string) (dogeboxd.ManifestSourceConfiguration, error) {
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

outer:
	for _, pupLocation := range pupLocations {
		for _, filename := range REQUIRED_FILES {
			p := filepath.Join(pupLocation, filename)
			if _, err := os.Stat(p); os.IsNotExist(err) {
				continue outer
			}
		}

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

		logoBase64 := ""

		if manifest.Meta.LogoPath != "" {
			logoPath := filepath.Join(pupLocation, manifest.Meta.LogoPath)
			if _, err := os.Stat(logoPath); err == nil {
				logoData, err := os.ReadFile(logoPath)
				if err == nil {
					logoBase64, err = utils.ImageBytesToWebBase64(logoData, manifest.Meta.LogoPath)
					if err != nil {
						// Don't fail if we can't read/convert the logo for whatever reason.
						log.Printf("failed to read/convert logo for %s: %s", manifest.Meta.Name, err)
					}
				}
			}
		}

		pup := dogeboxd.ManifestSourcePup{
			Name: manifest.Meta.Name,
			Location: map[string]string{
				"path": pupLocation,
			},
			Version:    manifest.Meta.Version,
			Manifest:   manifest,
			LogoBase64: logoBase64,
		}

		pups = append(pups, pup)
	}

	return dogeboxd.ManifestSourceList{
		Config:      r.config,
		LastChecked: time.Now(),
		Pups:        pups,
	}, nil
}

func (r ManifestSourceDisk) Download(diskPath string, remoteLocation map[string]string) error {
	sourcePath := remoteLocation["path"]

	// Copy the subpath to the final destination
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		destPath := filepath.Join(diskPath, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open source file: %w", err)
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create destination file: %w", err)
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return fmt.Errorf("failed to copy file contents: %w", err)
		}

		return os.Chmod(destPath, info.Mode())
	})

	if err != nil {
		return fmt.Errorf("failed to copy subpath to destination: %w", err)
	}

	return nil
}
