package repository

import (
	"errors"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
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

func (rm repositoryManager) GetRepositories() []dogeboxd.ManifestRepository {
	return rm.repositories
}

func (rm repositoryManager) AddRepository(repo dogeboxd.ManifestRepositoryConfiguration) (dogeboxd.ManifestRepository, error) {
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
