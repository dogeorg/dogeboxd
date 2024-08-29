package dogeboxd

/* Actions are passed to the dogeboxd service via its
 * AddAction method, and represent tasks that need to
 * be done such as installing a package, starting or
 * stopping a service etc.
 */
type Action any

// Install a pup on the system
type InstallPup struct {
	PupName    string
	PupVersion string
	SourceName string
}

// uninstall a pup
type UninstallPup struct {
	PupID string
}

// Enable a previously disabled pup
type EnablePup struct {
	PupID string
}

// Disable (stop) a running pup
type DisablePup struct {
	PupID string
}

// Updates the config values in a PUPStatus object
type UpdatePupConfig struct {
	PupID   string
	Payload map[string]string
}

type UpdatePendingSystemNetwork struct {
	Network SelectedNetwork
}
