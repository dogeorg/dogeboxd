package pup

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/utils"
)

/* This method is used to add a new pup from a manifest
* and init it's values to then be configured by the user
* and dogebox system. See PurgePup() for the opposite.
*
* Once a pup has been initialised it is considered 'managed'
* by the PupManager until purged.
*
* Returns PupID, error
 */
func (t PupManager) AdoptPup(m dogeboxd.PupManifest, source dogeboxd.ManifestSource, options dogeboxd.AdoptPupOptions) (string, error) {
	// Firstly (for now), check if we already have this manifest installed
	for _, p := range t.state {
		if m.Meta.Name == p.Manifest.Meta.Name && m.Meta.Version == p.Manifest.Meta.Version && p.Source.ID == source.Config().ID {
			return p.ID, dogeboxd.ErrPupAlreadyExists
		}
	}

	// Create a PupID for this new Pup
	var PupID string
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return PupID, err
	}
	PupID = fmt.Sprintf("%x", b)

	// Claim the next available IP
	for i := 3; i >= 0; i-- {
		t.lastIP[i]++
		if t.lastIP[i] > 0 {
			break
		}
		// If this octet wrapped, reset it to 0
		t.lastIP[i] = 0
	}

	// Check if we have gone off the edge of the world
	if t.lastIP[0] > 10 || (t.lastIP[0] == 10 && t.lastIP[1] > 70) {
		return PupID, errors.New("exhausted 65,534 IP addresses, what are you doing??")
	}

	// Create any WebUIs listed as exposed
	uis := []dogeboxd.PupWebUI{}
	for _, ex := range m.Container.Exposes {
		if ex.WebUI {
			uis = append(uis, dogeboxd.PupWebUI{
				Name:     ex.Name,
				Internal: ex.Port,
			})
		}
	}

	// and give them all Ports
	if len(uis) > 0 {
		ports := t.nextAvailablePorts(len(uis))
		for i := range uis {
			uis[i].Port = ports[0]
			ports = ports[1:]
		}
	}

	devModeServices := []string{}
	sourceConfig := source.Config()

	if sourceConfig.Type == "disk" {
		devModeServices, err = utils.GetPupNixDevelopmentModeServices(t.config, sourceConfig.Location, PupID, m)
		if err != nil {
			return PupID, err
		}
	}

	isDevModeAvailable := len(devModeServices) > 0

	if options.DevMode && !isDevModeAvailable {
		return PupID, errors.New("development mode is not available for this pup")
	}

	defaultConfig, err := dogeboxd.ExtractManifestConfigDefaults(m.Config)
	if err != nil {
		return PupID, fmt.Errorf("failed to apply config defaults: %w", err)
	}
	if defaultConfig == nil {
		defaultConfig = map[string]string{}
	}

	// Set up initial PupState and save it to disk
	p := dogeboxd.PupState{
		ID:           PupID,
		Source:       sourceConfig,
		Manifest:     m,
		Config:       defaultConfig,
		Installation: dogeboxd.STATE_INSTALLING,
		Enabled:      false,
		NeedsConf:    dogeboxd.ManifestConfigNeedsValues(m.Config, defaultConfig),
		NeedsDeps:    false, // TODO
		IP:           t.lastIP.String(),
		Version:      m.Meta.Version,
		WebUIs:       uis,

		IsDevModeEnabled: options.DevMode,
		DevModeServices:  devModeServices,
	}

	// Now save it to disk
	err = t.savePup(&p)
	if err != nil {
		return PupID, err
	}

	// If we've successfully saved to disk, set up in-memory.
	t.indexPup(&p)

	// update health details
	t.healthCheckPupState(&p)

	// Send a Pupdate announcing 'adopted'
	t.sendPupdate(dogeboxd.Pupdate{
		ID:    PupID,
		Event: dogeboxd.PUP_ADOPTED,
		State: p,
	})
	// Done! Adpoted
	return PupID, nil
}

/* Updating a PupState follows the veradic update func pattern
* to accept multiple types of updates before saving to disk as
* an atomic update.
*
* ie: err := manager.UpdatePup(id, SetPupInstallation(STATE_READY))
* see bottom of file for options
 */
func (t PupManager) UpdatePup(id string, updates ...func(*dogeboxd.PupState, *[]dogeboxd.Pupdate)) (dogeboxd.PupState, error) {
	p, ok := t.state[id]
	if !ok {
		return dogeboxd.PupState{}, dogeboxd.ErrPupNotFound
	}

	// capture any pupdates from updateFns
	pupdates := []dogeboxd.Pupdate{}
	for _, updateFn := range updates {
		updateFn(p, &pupdates)
	}

	// update pup healthcheck details before saving
	t.healthCheckPupState(p)

	// send any pupdates
	for _, pu := range pupdates {
		t.sendPupdate(pu)
	}

	return *p, t.savePup(p)
}

func (t PupManager) PurgePup(pupId string) error {
	// Get the pup state before removing it so we can send the event
	pup, exists := t.state[pupId]

	// Remove our in-memory state
	delete(t.state, pupId)
	delete(t.stats, pupId)

	// Send a Pupdate announcing 'purged' after removal
	if exists {
		t.sendPupdate(dogeboxd.Pupdate{
			ID:    pupId,
			Event: dogeboxd.PUP_PURGED,
			State: *pup,
		})
	}

	return nil
}

func (t PupManager) indexPup(p *dogeboxd.PupState) {
	systemMetrics := []dogeboxd.PupMetrics[any]{
		{
			Name:   "CPU",
			Label:  "CPU",
			Type:   "float",
			Values: dogeboxd.NewBuffer[any](30),
		},
		{
			Name:   "Memory",
			Label:  "Memory",
			Type:   "float",
			Values: dogeboxd.NewBuffer[any](30),
		},
		{
			Name:   "Memory Percent",
			Label:  "Memory Percent",
			Type:   "float",
			Values: dogeboxd.NewBuffer[any](30),
		},
		{
			Name:   "Disk Usage",
			Label:  "Disk Usage",
			Type:   "float",
			Values: dogeboxd.NewBuffer[any](30),
		},
	}

	metrics := []dogeboxd.PupMetrics[any]{}

	// handle custom metrics defined in manifest
	for _, m := range p.Manifest.Metrics {
		if m.Name == "" || m.HistorySize <= 0 {
			fmt.Println("Manifest metric has invalid fields", m)
			continue
		}

		metric := dogeboxd.PupMetrics[any]{
			Name:   m.Name,
			Label:  m.Label,
			Type:   m.Type,
			Values: dogeboxd.NewBuffer[any](m.HistorySize),
		}

		metrics = append(metrics, metric)
	}

	s := dogeboxd.PupStats{
		ID:            p.ID,
		Status:        dogeboxd.STATE_STOPPED,
		SystemMetrics: systemMetrics,
		Metrics:       metrics,
	}

	t.state[p.ID] = p
	t.stats[p.ID] = &s
}

// isPortAvailable checks if a port is actually available on the system
func (t PupManager) isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// get N available webUI ports. These must be set on
// a PupState before you can call again without getting
// duplicates
func (t PupManager) nextAvailablePorts(howMany int) []int {
	if howMany <= 0 {
		return []int{}
	}
	out := []int{}
	consumed := map[int]struct{}{} // track already used ports

	// find all current ports
	for _, ps := range t.state {
		// any ports already assigned to WebUIs
		for _, w := range ps.WebUIs {
			consumed[w.Port] = struct{}{}
		}
		// and any ports Exposed by the manifest on the host
		for _, ex := range ps.Manifest.Container.Exposes {
			if ex.ListenOnHost {
				consumed[ex.Port] = struct{}{}
			}
		}
	}

	for len(out) < howMany {
		fmt.Println("filled ports", len(out))
		for port := MIN_WEBUI_PORT; true; port++ {
			fmt.Println("PORT", port)
			// check port not in use anywhere
			_, exists := consumed[port]
			if !exists && t.isPortAvailable(port) {
				out = append(out, port)
				consumed[port] = struct{}{}
				fmt.Printf("Allocated port %d (available on system)\n", port)
				break
			}
		}
	}
	fmt.Println("sending needed ports", out)
	return out
}
