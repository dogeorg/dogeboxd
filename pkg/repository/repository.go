package repository

import (
	"errors"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var REQUIRED_FILES = []string{"pup.nix", "manifest.json"}

func NewRepositoryFromConfiguration(config dogeboxd.ManifestRepositoryConfiguration) (dogeboxd.ManifestRepository, error) {
	var repository dogeboxd.ManifestRepository

	switch config.Type {
	case "local":
		repository = ManifestRepositoryDisk{config: config}
	case "git":
		repository = ManifestRepositoryGit{config: config}

	default:
		return nil, fmt.Errorf("unknown repository type: %s", config.Type)
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
