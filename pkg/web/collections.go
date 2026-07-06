package web

import (
	"fmt"
	"log"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Masterminds/semver/v3"
)

// CollectionPup represents a pup that belongs to a collection
type CollectionPup struct {
	Name     string
	Version  string
	SourceId string
}

// FoundationPupCollections maps collection names to their respective pups
var FoundationPupCollections = map[string][]CollectionPup{
	"core": {
		{
			Name:     "Dogecoin Core",
			Version:  "latest",
			SourceId: "dogeorg.pups",
		},
	},
	"essentials": {
		{
			Name:     "Dogecoin Core",
			Version:  "latest",
			SourceId: "dogeorg.pups",
		},
		{
			Name:     "Dogenet",
			Version:  "latest",
			SourceId: "dogeorg.pups",
		},
		{
			Name:     "Dogemap",
			Version:  "latest",
			SourceId: "dogeorg.pups",
		},
		{
			Name:     "Identity",
			Version:  "latest",
			SourceId: "dogeorg.pups",
		},
	},
	"custom": {},
}

// processPupCollections handles the installation of pups for a given collection
func processPupCollections(sourceManager dogeboxd.SourceManager, dbx dogeboxd.Dogeboxd, sessionToken string, collectionName string) {
	// Get the list of pups for the selected collection
	pupsToInstall, exists := FoundationPupCollections[collectionName]
	if !exists {
		log.Printf("Unknown collection: %s", collectionName)
		return
	}

	// Create a batch installation request
	allSources, err := sourceManager.GetAll(false)
	if err != nil {
		log.Printf("Failed to fetch sources for collection %q: %v", collectionName, err)
		return
	}

	installRequests := []dogeboxd.InstallPup{}
	for _, pup := range pupsToInstall {
		resolvedVersion, err := resolveVersionConstraint(allSources, pup.SourceId, pup.Name, pup.Version)
		if err != nil {
			log.Printf("Skipping collection pup %q: %v", pup.Name, err)
			continue
		}

		log.Printf("Collection version resolved: %s %s -> %s", pup.Name, pup.Version, resolvedVersion)

		installRequests = append(installRequests, dogeboxd.InstallPup{
			PupName:      pup.Name,
			PupVersion:   resolvedVersion,
			SourceId:     pup.SourceId,
			Options:      dogeboxd.AdoptPupOptions{},
			SessionToken: sessionToken,
		})
	}

	if len(installRequests) == 0 {
		log.Printf("No pups from collection %q resolved successfully", collectionName)
		return
	}

	// Add the batch installation action
	dbx.AddAction(dogeboxd.InstallPups(installRequests))
}

// resolveVersionConstraint resolves a version constraint against available pups.
// Supported formats: "latest", exact semver ("1.6.7"), tilde ("~1.6.0"), caret ("^2.8.0").
func resolveVersionConstraint(allSources map[string]dogeboxd.ManifestSourceList, sourceID, pupName, rawConstraint string) (string, error) {
	constraint := strings.TrimSpace(rawConstraint)
	if constraint == "" {
		return "", fmt.Errorf("empty version constraint for pup %q", pupName)
	}

	switch {
	case constraint == "latest":
		constraint = "*"
	case strings.HasPrefix(constraint, "^"), strings.HasPrefix(constraint, "~"):
		// allowed range forms
	default:
		if _, err := semver.NewVersion(constraint); err != nil {
			return "", fmt.Errorf("unsupported version constraint %q for pup %q: use latest, exact semver, or ~/^ ranges", rawConstraint, pupName)
		}
	}

	parsedConstraint, err := semver.NewConstraint(constraint)
	if err != nil {
		return "", fmt.Errorf("invalid version constraint %q for pup %q: %w", rawConstraint, pupName, err)
	}

	sourceList, found := allSources[sourceID]
	if !found {
		return "", fmt.Errorf("source %q not found", sourceID)
	}

	var highestMatch *semver.Version
	for _, sourcePup := range sourceList.Pups {
		if sourcePup.Name != pupName {
			continue
		}
		v, err := semver.NewVersion(sourcePup.Version)
		if err != nil {
			continue
		}
		if parsedConstraint.Check(v) && (highestMatch == nil || v.GreaterThan(highestMatch)) {
			highestMatch = v
		}
	}

	if highestMatch == nil {
		return "", fmt.Errorf("no matching version for pup %q in source %q with constraint %q", pupName, sourceID, rawConstraint)
	}

	return highestMatch.Original(), nil
}
