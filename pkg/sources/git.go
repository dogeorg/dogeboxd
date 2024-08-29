package source

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

var _ dogeboxd.ManifestSource = &ManifestSourceGit{}

type ManifestSourceGit struct {
	config    dogeboxd.ManifestSourceConfiguration
	_cache    dogeboxd.ManifestSourceList
	_isCached bool
}

func (r ManifestSourceGit) Name() string {
	return r.config.Name
}

func (r ManifestSourceGit) Config() dogeboxd.ManifestSourceConfiguration {
	return r.config
}

func (r ManifestSourceGit) getShallowWorktree(tag string) (*git.Worktree, *git.Repository, error) {
	storage := memory.NewStorage()
	fs := memfs.New()

	// Clone the repository with the specific tag
	repo, err := git.Clone(storage, fs, &git.CloneOptions{
		URL:           r.config.Location,
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

func (r ManifestSourceGit) ensureTagValidAndGetManifest(tag string) (pup.PupManifest, bool, error) {
	worktree, _, err := r.getShallowWorktree(tag)
	if err != nil {
		return pup.PupManifest{}, false, err
	}

	for _, filename := range REQUIRED_FILES {
		_, err := worktree.Filesystem.Stat(filename)
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("tag %s missing file %s", tag, filename)
				return pup.PupManifest{}, false, nil
			}
			return pup.PupManifest{}, false, fmt.Errorf("failed to check for file %s: %w", filename, err)
		}
	}

	content, err := worktree.Filesystem.Open("manifest.json")
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

	log.Printf("Successfully read manifest for tag %s", tag)
	return manifest, true, nil
}

func (r ManifestSourceGit) Validate() (bool, error) {
	if err := validateGitRemoteURL(r.config.Location); err != nil {
		return false, fmt.Errorf("invalid git remote URL: %w", err)
	}

	repo, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:          r.config.Location,
		Depth:        1,
		SingleBranch: true,
		Tags:         git.AllTags,
	})

	if err != nil {
		return false, err
	}

	// Check if our main branch has a manifest file.
	// If it doesn't, don't even bother checking any tags below.
	worktree, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	_, err = worktree.Filesystem.Stat("manifest.json")
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("missing root manifest.json")
		}
		return false, fmt.Errorf("failed to check for root manifest.json: %w", err)
	}

	return true, nil
}

func (r ManifestSourceGit) List(ignoreCache bool) (dogeboxd.ManifestSourceList, error) {
	if !ignoreCache && r._isCached {
		return r._cache, nil
	}

	// At the moment we only support a single pup per repository.
	// This will change in the future with the introduction of a root
	// dogebox.json or something that can point to sub-pups.
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

	type tagResult struct {
		version  string
		valid    bool
		manifest pup.PupManifest
		err      error
	}

	resultChan := make(chan tagResult)
	var tagCount int

	err = iter.ForEach(func(p *plumbing.Reference) error {
		tagRef := string(p.Name())
		tagName := strings.Replace(tagRef, "refs/tags/", "", -1)
		if semver.IsValid(tagName) {
			tagCount++
			go func(ref, version string) {
				pupManifest, isValid, err := r.ensureTagValidAndGetManifest(ref)
				resultChan <- tagResult{version: version, valid: isValid, manifest: pupManifest, err: err}
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
		if result.valid {
			validPups = append(validPups, dogeboxd.ManifestSourcePup{
				Name:     r.config.Name,
				Location: result.version,
				Version:  result.manifest.Meta.Version,
				Manifest: result.manifest,
			})
		} else {
			log.Printf("Found valid semver tag %s but tag is missing required files", result.version)
		}
	}

	list := dogeboxd.ManifestSourceList{
		LastUpdated: time.Now(),
		Pups:        validPups,
	}

	r._cache = list
	r._isCached = true

	return r._cache, nil
}

func (r ManifestSourceGit) Download(diskPath string, location string) error {
	// At the moment we only support a single pup per repository,
	// so we can ignore the name field here, eventually it will be used.
	// For a disk repository, we always just return what is on-disk, unversioned.
	_, err := git.PlainClone(diskPath, false, &git.CloneOptions{
		URL:           r.config.Location,
		ReferenceName: plumbing.ReferenceName("refs/tags/" + location),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return err
	}

	return nil
}

func validateGitRemoteURL(urlStr string) error {
	// Parse the URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// Check the scheme
	switch u.Scheme {
	case "http", "https", "git", "ssh":
		// These are valid schemes for git
	case "":
		// If no scheme is provided, it might be an SSH URL
		if strings.Contains(u.Path, ":") {
			// Looks like a valid SSH URL (e.g., git@github.com:user/repo.git)
			return nil
		}
		return fmt.Errorf("missing scheme in URL")
	default:
		return fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}

	// Ensure there's a host
	if u.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Ensure there's a path (which would be the repository)
	if u.Path == "" || u.Path == "/" {
		return fmt.Errorf("missing repository path in URL")
	}

	return nil
}
