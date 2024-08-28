package repository

import (
	"errors"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var REQUIRED_FILES = []string{"pup.nix", "manifest.json"}

func NewRepositoryManager(sm dogeboxd.StateManager) dogeboxd.RepositoryManager {
	state := sm.Get().Repository

	return repositoryManager{
		repositories: state.Repositories,
	}
}

var _ dogeboxd.RepositoryManager = &repositoryManager{}

type repositoryManager struct {
	sm           dogeboxd.StateManager
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

	return repository, nil
}
