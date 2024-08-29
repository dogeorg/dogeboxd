package repository

import (
	"errors"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

var REQUIRED_FILES = []string{"pup.nix", "manifest.json"}

func NewRepositoryManager(sm dogeboxd.StateManager, pm dogeboxd.PupManager) dogeboxd.RepositoryManager {
	state := sm.Get().Repository

	return repositoryManager{
		sm:           sm,
		pm:           pm,
		repositories: state.Repositories,
	}
}

var _ dogeboxd.RepositoryManager = &repositoryManager{}

type repositoryManager struct {
	sm           dogeboxd.StateManager
	pm           dogeboxd.PupManager
	repositories []dogeboxd.ManifestRepository
}

func (rm repositoryManager) GetAll() (map[string]dogeboxd.ManifestRepositoryList, error) {
	available := map[string]dogeboxd.ManifestRepositoryList{}

	for _, r := range rm.repositories {
		l, err := r.List(false)
		if err != nil {
			return nil, err
		}

		available[r.Name()] = l
	}

	return available, nil
}

func (rm repositoryManager) GetRepositoryManifest(repositoryName, pupName, pupVersion string) (pup.PupManifest, error) {
	for _, r := range rm.repositories {
		if r.Name() == repositoryName {
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

	return pup.PupManifest{}, fmt.Errorf("no repository found with name %s", repositoryName)
}

func (rm repositoryManager) GetRepositoryPup(repositoryName, pupName, pupVersion string) (dogeboxd.ManifestRepositoryPup, error) {
	r, err := rm.GetRepository(repositoryName)
	if err != nil {
		return dogeboxd.ManifestRepositoryPup{}, err
	}

	l, err := r.List(false)
	if err != nil {
		return dogeboxd.ManifestRepositoryPup{}, err
	}

	for _, pup := range l.Pups {
		if pup.Name == pupName && pup.Version == pupVersion {
			return pup, nil
		}
	}

	return dogeboxd.ManifestRepositoryPup{}, fmt.Errorf("no pup found with name %s and version %s", pupName, pupVersion)
}

func (rm repositoryManager) GetRepository(name string) (dogeboxd.ManifestRepository, error) {
	for _, r := range rm.repositories {
		if r.Name() == name {
			return r, nil
		}
	}

	return nil, fmt.Errorf("no repository found with name %s", name)
}

func (rm repositoryManager) DownloadPup(path, repositoryName, pupName, pupVersion string) error {
	r, err := rm.GetRepository(repositoryName)
	if err != nil {
		return err
	}

	repoPup, err := rm.GetRepositoryPup(repositoryName, pupName, pupVersion)

	return r.Download(path, repoPup.Location)
}

func (rm repositoryManager) AddRepository(repo dogeboxd.ManifestRepositoryConfiguration) (dogeboxd.ManifestRepository, error) {
	// Ensure no existing repository has the same name
	for _, r := range rm.repositories {
		if r.Name() == repo.Name {
			return nil, fmt.Errorf("repository with name %s already exists", repo.Name)
		}
	}

	var repository dogeboxd.ManifestRepository

	switch repo.Type {
	case "local":
		repository = ManifestRepositoryDisk{config: repo}
	case "git":
		repository = ManifestRepositoryGit{config: repo}

	default:
		return nil, fmt.Errorf("unknown repository type: %s", repo.Type)
	}

	valid, err := repository.Validate()

	if err != nil {
		log.Println("error while validating repository:", err)
		return nil, err
	}

	if !valid {
		return nil, errors.New("repository failed to validate")
	}

	rm.repositories = append(rm.repositories, repository)

	state := rm.sm.Get().Repository
	state.Repositories = rm.repositories
	rm.sm.SetRepository(state)
	if err := rm.sm.Save(); err != nil {
		return nil, err
	}

	return repository, nil
}

func (rm repositoryManager) RemoveRepository(name string) error {
	var matched dogeboxd.ManifestRepository
	var matchedIndex int

	for i, r := range rm.repositories {
		if r.Name() == name {
			matched = r
			matchedIndex = i
		}
	}

	if matched == nil {
		return fmt.Errorf("no existing repository named %s", name)
	}

	// Check if we have an existing pup that is from
	// this repository if we do, we don't let removal happen.
	installedPups := rm.pm.GetAllFromSource(matched.Config())

	if len(installedPups) != 0 {
		return fmt.Errorf("%d installed pups using this source, aborting", len(installedPups))
	}

	rm.repositories = append(rm.repositories[:matchedIndex], rm.repositories[matchedIndex+1:]...)

	state := rm.sm.Get().Repository
	state.Repositories = rm.repositories
	rm.sm.SetRepository(state)
	if err := rm.sm.Save(); err != nil {
		return err
	}

	return nil
}
