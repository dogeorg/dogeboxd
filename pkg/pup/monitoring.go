package pup

import (
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// called when we expect a pup to be changing state,
// this will rapidly poll for a few seconds and update
// the frontend with status.
func (t PupManager) FastPollPup(id string) {
	t.monitor.GetFastMonChannel() <- fmt.Sprintf("container@pup-%s.service", id)
}

/* Set the list of monitored services on the SystemMonitor */
func (t PupManager) updateMonitoredPups() {
	serviceNames := []string{}
	for _, p := range t.state {
		if p.Installation == dogeboxd.STATE_READY {
			serviceNames = append(serviceNames, fmt.Sprintf("container@pup-%s.service", p.ID))
		}
	}
	t.monitor.GetMonChannel() <- serviceNames
}
