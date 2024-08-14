package dogeboxd

import "errors"

/* Actions are passed to the dogeboxd service via its
 * AddAction method, and represent tasks that need to
 * be done such as installing a package, starting or
 * stopping a service etc.
 */
type Action any

/* A type of action that has a PupID that needs
 * to be enriched with a related Pup Manifest
 * before processing
 */
type PupAction struct {
	PupID string
	M     *PupManifest
}

func (t *PupAction) LoadManifest(mi ManifestIndex) error {
	m, ok := mi.FindManifest(t.PupID)
	if !ok {
		return errors.New("couldn't find manifest for action")
	}
	t.M = &m
	return nil
}

// Install a pup on the system
type InstallPup struct {
	PupAction
	PupID string
	M     *PupManifest
}

// uninstall a pup
type UninstallPup struct {
	PupAction
	PupID string
	M     *PupManifest
}

// Enable a previously disabled pup
type EnablePup struct {
	PupAction
	PupID string
	M     *PupManifest
}

// Disable (stop) a running pup
type DisablePup struct {
	PupAction
	PupID string
	M     *PupManifest
}

// Updates the config values in a PUPStatus object
type UpdatePupConfig struct {
	PupID   string
	Payload map[string]string
}
