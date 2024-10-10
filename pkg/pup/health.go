package pup

import (
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// This function only checks pup-specific conditions, it does not check
// the rest of the system is ready for a pup to start.
func (t PupManager) CanPupStart(pupId string) (bool, error) {
	pup, ok := t.state[pupId]
	if !ok {
		return false, dogeboxd.ErrPupNotFound
	}

	report := t.GetPupHealthState(pup)

	// If we still need config or deps, don't start.
	if report.NeedsConf || report.NeedsDeps {
		return false, nil
	}

	// TODO: This doesn't work when being called from our dbx CLI
	//       as our system updates aren't running.

	// If a dep isn't running, don't start.
	// if len(report.Issues.DepsNotRunning) > 0 {
	// 	return false, nil
	// }

	return true, nil
}

func (t PupManager) GetPupHealthState(pup *dogeboxd.PupState) dogeboxd.PupHealthStateReport {
	// are our required config fields set?
	configSet := true
loop:
	for _, section := range pup.Manifest.Config.Sections {
		for _, field := range section.Fields {
			if field.Required {
				_, ok := pup.Config[field.Name]
				if !ok {
					configSet = false
					break loop
				}
			}
		}
	}

	// are our deps met?
	depsMet := true
	depsNotRunning := []string{}
	for _, d := range t.calculateDeps(pup) {
		depMet := false
		for iface, pupID := range pup.Providers {
			if d.Interface == iface {
				depMet = true
				provPup, ok := t.stats[pupID]
				if !ok {
					depMet = false
					fmt.Printf("pup %s missing, but provides %s to %s", pupID, iface, pup.ID)
				} else {
					if provPup.Status != dogeboxd.STATE_RUNNING {
						depsNotRunning = append(depsNotRunning, iface)
					}
				}
			}
		}
		if !depMet {
			depsMet = false
		}
	}

	report := dogeboxd.PupHealthStateReport{
		Issues: dogeboxd.PupIssues{
			DepsNotRunning: depsNotRunning,
			// TODO: HealthWarnings
			// TODO: UpdateAvailable
		},
		NeedsConf: !configSet,
		NeedsDeps: !depsMet,
	}

	return report
}

// Modify provided pup to update warning flags
func (t PupManager) healthCheckPupState(pup *dogeboxd.PupState) {
	report := t.GetPupHealthState(pup)

	pup.NeedsConf = report.NeedsConf
	pup.NeedsDeps = report.NeedsDeps
	t.stats[pup.ID].Issues = report.Issues
}
