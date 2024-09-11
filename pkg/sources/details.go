package source

import (
	"encoding/json"
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func ParseAndValidateSourceDetails(content string) (dogeboxd.SourceDetails, error) {
	var details dogeboxd.SourceDetails
	err := json.Unmarshal([]byte(content), &details)
	if err != nil {
		return dogeboxd.SourceDetails{}, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	// Check we have an ID and a name, description is optional.
	// Empty/missing pups are fine, but check any defined ones have a location.

	if details.ID == "" {
		return dogeboxd.SourceDetails{}, fmt.Errorf("missing field: id")
	}

	if details.Name == "" {
		return dogeboxd.SourceDetails{}, fmt.Errorf("missing field: name")
	}

	for _, pup := range details.Pups {
		if pup.Location == "" {
			return dogeboxd.SourceDetails{}, fmt.Errorf("missing field: location in pups")
		}
	}

	return details, nil
}
