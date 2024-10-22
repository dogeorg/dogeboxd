package pup

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t PupManager) CalculateDeps(pupID string) ([]dogeboxd.PupDependencyReport, error) {
	pup, ok := t.state[pupID]
	if !ok {
		return []dogeboxd.PupDependencyReport{}, errors.New("no such pup")
	}
	return t.calculateDeps(pup), nil
}

// This function calculates a DependencyReport for every
// dep that a given pup requires
func (t PupManager) calculateDeps(pupState *dogeboxd.PupState) []dogeboxd.PupDependencyReport {
	deps := []dogeboxd.PupDependencyReport{}
	for _, dep := range pupState.Manifest.Dependencies {
		report := dogeboxd.PupDependencyReport{
			Interface: dep.InterfaceName,
			Version:   dep.InterfaceVersion,
			Optional:  dep.Optional,
		}

		constraint, err := semver.NewConstraint(dep.InterfaceVersion)
		if err != nil {
			fmt.Printf("Invalid version constraint: %s, %s:%s\n", pupState.Manifest.Meta.Name, dep.InterfaceName, dep.InterfaceVersion)
			deps = append(deps, report)
			continue
		}

		// Is there currently a provider set?
		report.CurrentProvider = pupState.Providers[dep.InterfaceName]

		// What are all installed pups that can provide the interface?
		installed := []string{}
		for id, p := range t.state {
			// search the interfaces and check against constraint
			for _, iface := range p.Manifest.Interfaces {
				ver, err := semver.NewVersion(iface.Version)
				if err != nil {
					continue
				}
				if iface.Name == dep.InterfaceName && constraint.Check(ver) == true {
					installed = append(installed, id)
				}
			}
		}
		report.InstalledProviders = installed

		// What are all available pups that can provide the interface?
		available := []dogeboxd.PupManifestDependencySource{}
		sourceList, err := t.sourceManager.GetAll(false)
		if err == nil {
			for _, list := range sourceList {
				// search the interfaces and check against constraint
				for _, p := range list.Pups {
					for _, iface := range p.Manifest.Interfaces {
						ver, err := semver.NewVersion(iface.Version)
						if err != nil {
							continue
						}
						if iface.Name == dep.InterfaceName && constraint.Check(ver) == true {
							// check if this isnt alread installed..
							alreadyInstalled := false
							for _, installedPupID := range installed {
								iPup, _, err := t.GetPup(installedPupID)
								if err != nil {
									continue
								}
								if iPup.Source.Location == list.Config.Location && iPup.Manifest.Meta.Name == p.Name {
									// matching location and name, assume already installed
									alreadyInstalled = true
									break
								}
							}

							if !alreadyInstalled {
								available = append(available, dogeboxd.PupManifestDependencySource{
									SourceLocation: list.Config.Location,
									PupName:        p.Name,
									PupVersion:     p.Version,
									PupLogoBase64:  p.LogoBase64,
								})
							}
						}
					}
				}
			}
			report.InstallableProviders = available
		}

		// Is there a DefaultSourceProvider
		report.DefaultSourceProvider = dep.DefaultSource

		deps = append(deps, report)
	}
	return deps
}
