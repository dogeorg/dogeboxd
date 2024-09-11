package source

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

var _ dogeboxd.ManifestSource = &ManifestSourceGit{}

type ManifestSourceGit struct {
	serverConfig dogeboxd.ServerConfig
	config       dogeboxd.ManifestSourceConfiguration
	_cache       dogeboxd.ManifestSourceList
	_isCached    bool
}

func (r ManifestSourceGit) ValidateFromLocation(location string) (dogeboxd.ManifestSourceConfiguration, error) {
	// Get all our tags for this repository.
	tags, err := r.GetAllGitTags(location)
	if err != nil {
		return dogeboxd.ManifestSourceConfiguration{}, err
	}

	// Filter out non-semver tags and find the greatest version
	var validTags []string
	for _, tag := range tags {
		if semver.IsValid(tag) {
			validTags = append(validTags, tag)
		}
	}

	if len(validTags) == 0 {
		return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("no valid semver tags found")
	}

	semver.Sort(validTags)
	latestVersion := validTags[len(validTags)-1]

	details, valid, err := r.getSourceDetails(location, "refs/tags/"+latestVersion)

	if err != nil {
		log.Printf("Error getting source details: %v", err)
		return dogeboxd.ManifestSourceConfiguration{}, err
	}

	if valid {
		return dogeboxd.ManifestSourceConfiguration{
			ID:          details.ID,
			Name:        details.Name,
			Description: details.Description,
			Location:    location,
			Type:        "git",
		}, nil
	}

	// If we don't have a valid dogebox.json, check if this is a root-level pup.
	worktree, _, err := r.getShallowWorktree(location, "refs/tags/"+latestVersion)
	if err != nil {
		return dogeboxd.ManifestSourceConfiguration{}, err
	}

	manifestFile, err := worktree.Filesystem.Open("manifest.json")
	if err != nil {
		if os.IsNotExist(err) {
			return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("manifest.json not found in the root of the repository")
		}
		return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("error opening manifest.json: %w", err)
	}
	defer manifestFile.Close()

	var manifest pup.PupManifest
	decoder := json.NewDecoder(manifestFile)
	if err := decoder.Decode(&manifest); err != nil {
		return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("error parsing manifest.json: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("invalid manifest.json: %w", err)
	}

	var sourceId string
	b := make([]byte, 16)
	_, err = rand.Read(b)
	if err != nil {
		return dogeboxd.ManifestSourceConfiguration{}, err
	}
	sourceId = fmt.Sprintf("%x", b)

	return dogeboxd.ManifestSourceConfiguration{
		ID:          sourceId,
		Name:        manifest.Meta.Name,
		Description: "",
		Location:    location,
		Type:        "git",
	}, nil
}

func (r ManifestSourceGit) GetAllGitTags(location string) ([]string, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{location},
	})

	refs, err := rem.List(&git.ListOptions{
		PeelingOption: git.AppendPeeled,
	})
	if err != nil {
		return []string{}, err
	}

	// Filters the references list and only keeps tags
	var tags []string
	for _, ref := range refs {
		if ref.Name().IsTag() {
			tags = append(tags, ref.Name().Short())
		}
	}

	return tags, nil
}

func (r ManifestSourceGit) Name() string {
	return r.config.Name
}

func (r ManifestSourceGit) Config() dogeboxd.ManifestSourceConfiguration {
	return r.config
}

func (r ManifestSourceGit) getShallowWorktree(location, tag string) (*git.Worktree, *git.Repository, error) {
	storage := memory.NewStorage()
	fs := memfs.New()

	// Clone the repository with the specific tag
	repo, err := git.Clone(storage, fs, &git.CloneOptions{
		URL:           location,
		ReferenceName: plumbing.ReferenceName(tag),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return &git.Worktree{}, &git.Repository{}, fmt.Errorf("failed to clone repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &git.Worktree{}, &git.Repository{}, fmt.Errorf("failed to get worktree: %w", err)
	}

	return worktree, repo, nil
}

func (r ManifestSourceGit) getSourceDetails(location, tag string) (dogeboxd.SourceDetails, bool, error) {
	worktree, _, err := r.getShallowWorktree(location, tag)
	if err != nil {
		log.Printf("Error getting shallow worktree: %v", err)
		return dogeboxd.SourceDetails{}, false, err
	}

	return r.getSourceDetailsFromWorktree(worktree)
}

func (r ManifestSourceGit) getSourceDetailsFromWorktree(worktree *git.Worktree) (dogeboxd.SourceDetails, bool, error) {
	indexPath := "dogebox.json"
	_, err := worktree.Filesystem.Stat(indexPath)
	if err == nil {
		content, err := worktree.Filesystem.Open(indexPath)
		if err != nil {
			return dogeboxd.SourceDetails{}, false, fmt.Errorf("failed to open dogebox.json: %w", err)
		}
		defer content.Close()

		manifestBytes, err := io.ReadAll(content)
		if err != nil {
			return dogeboxd.SourceDetails{}, false, fmt.Errorf("failed to read dogebox.json: %w", err)
		}

		d, err := ParseAndValidateSourceDetails(string(manifestBytes))
		if err != nil {
			return dogeboxd.SourceDetails{}, false, fmt.Errorf("failed to parse and validate dogebox.json: %w", err)
		}

		return d, true, nil
	}

	return dogeboxd.SourceDetails{}, false, nil
}

type GitPupEntry struct {
	Manifest pup.PupManifest
	SubPath  string
}

func (r ManifestSourceGit) ensureTagValidAndGetPups(tag string) ([]GitPupEntry, error) {
	entries := []GitPupEntry{}

	worktree, _, err := r.getShallowWorktree(r.config.Location, tag)
	if err != nil {
		return []GitPupEntry{}, err
	}

	pupLocations := []string{}

	tagDetails, foundDetails, err := r.getSourceDetailsFromWorktree(worktree)
	if err != nil {
		return []GitPupEntry{}, err
	}

	if foundDetails {
		for _, pup := range tagDetails.Pups {
			pupLocations = append(pupLocations, pup.Location)
		}
	} else {
		pupLocations = append(pupLocations, ".")
	}

	for _, pupLocation := range pupLocations {
		pupManifest, isValid, err := r.getPupManifestFromWorktreeLocation(tag, worktree, pupLocation)
		if err != nil {
			return []GitPupEntry{}, err
		}
		if isValid {
			entries = append(entries, GitPupEntry{
				Manifest: pupManifest,
				SubPath:  pupLocation,
			})
		}
	}

	return entries, nil
}

func (r ManifestSourceGit) getPupManifestFromWorktreeLocation(tag string, worktree *git.Worktree, location string) (pup.PupManifest, bool, error) {
	for _, filename := range REQUIRED_FILES {
		_, err := worktree.Filesystem.Stat(filepath.Join(location, filename))
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("tag %s missing file %s", tag, filename)
				return pup.PupManifest{}, false, nil
			}
			return pup.PupManifest{}, false, fmt.Errorf("failed to check for file %s: %w", filename, err)
		}
	}

	content, err := worktree.Filesystem.Open(filepath.Join(location, "manifest.json"))
	if err != nil {
		return pup.PupManifest{}, false, fmt.Errorf("failed to open manifest.json: %w", err)
	}
	defer content.Close()

	manifestBytes, err := io.ReadAll(content)
	if err != nil {
		return pup.PupManifest{}, false, fmt.Errorf("failed to read manifest.json: %w", err)
	}

	var manifest pup.PupManifest
	err = json.Unmarshal(manifestBytes, &manifest)
	if err != nil {
		return pup.PupManifest{}, false, fmt.Errorf("failed to unmarshal manifest.json: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return pup.PupManifest{}, false, fmt.Errorf("manifest validation failed: %w", err)
	}

	log.Printf("Successfully read manifest for location %s", location)
	return manifest, true, nil
}

func (r *ManifestSourceGit) List(ignoreCache bool) (dogeboxd.ManifestSourceList, error) {
	if !ignoreCache && r._isCached {
		return r._cache, nil
	}

	repo, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:          r.config.Location,
		Depth:        1,
		SingleBranch: true,
		Tags:         git.AllTags,
	})

	if err != nil {
		return dogeboxd.ManifestSourceList{}, err
	}

	iter, err := repo.Tags()
	if err != nil {
		return dogeboxd.ManifestSourceList{}, err
	}

	type TagResult struct {
		version string
		entries []GitPupEntry
		err     error
	}

	resultChan := make(chan TagResult)
	var tagCount int

	err = iter.ForEach(func(p *plumbing.Reference) error {
		tagRef := string(p.Name())
		tagName := strings.Replace(tagRef, "refs/tags/", "", -1)
		if semver.IsValid(tagName) {
			tagCount++
			go func(ref, version string) {
				entries, err := r.ensureTagValidAndGetPups(ref)
				resultChan <- TagResult{version: version, entries: entries, err: err}
			}(tagRef, tagName)
		}

		return nil
	})

	if err != nil {
		return dogeboxd.ManifestSourceList{}, err
	}

	validPups := []dogeboxd.ManifestSourcePup{}

	for i := 0; i < tagCount; i++ {
		result := <-resultChan
		if result.err != nil {
			log.Printf("Error validating tag %s: %v", result.version, result.err)
			continue
		}

		for _, entry := range result.entries {
			validPups = append(validPups, dogeboxd.ManifestSourcePup{
				Name: entry.Manifest.Meta.Name,
				Location: map[string]string{
					"tag":     result.version,
					"subPath": entry.SubPath,
				},
				Version:  entry.Manifest.Meta.Version,
				Manifest: entry.Manifest,
			})
		}
	}

	list := dogeboxd.ManifestSourceList{
		Config:      r.config,
		LastChecked: time.Now(),
		Pups:        validPups,
	}

	r._cache = list
	r._isCached = true

	return r._cache, nil
}

func (r ManifestSourceGit) Download(diskPath string, location map[string]string) error {
	tempDir, err := os.MkdirTemp(r.serverConfig.TmpDir, "pup-clone-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	log.Printf("Cloning repository %s (tag: %s) to temporary directory", r.config.Location, location["tag"])

	_, err = git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:           r.config.Location,
		ReferenceName: plumbing.ReferenceName("refs/tags/" + location["tag"]),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Construct the path to the subpath within the cloned repository
	sourcePath := filepath.Join(tempDir, location["subPath"])

	// Ensure the source path exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return fmt.Errorf("subpath %s does not exist in the cloned repository", location["subPath"])
	}

	// Copy the subpath to the final destination
	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
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

	log.Printf("Successfully downloaded and moved subpath %s to %s", location["subPath"], diskPath)

	return nil
}
