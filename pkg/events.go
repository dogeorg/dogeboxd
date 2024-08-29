package dogeboxd

// A Job is created when an Action is recieved by the system.
// Jobs are passed through the Dogeboxd service and result in
// a Change being send to the client via websockets.
type Job struct {
	A       Action
	ID      string
	Err     string
	Success any
	State   *PupState // nilable, check before use!
}

// A Change can be the result of a Job (same ID) or
// represent an internal system change originating
// from elsewhere.
//
// A Change encodes an 'update' (see below)
// A Change as the result of an Action may carry
// an 'error' to the frontend for the same Job ID
type Change struct {
	ID     string `json:"id"`
	Error  string `json:"error"`
	Type   string `json:"type"`
	Update Update `json:"update"`
}

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

/* Updates are responses to Actions or simply
* internal state changes that the frontend needs,
* these are wrapped in a 'change' and sent via
* websocket to the client.
*
* Updates need to be json-marshalable types
 */
type Update any

// StatsUpdate represents one or more PupStats updates
type StatsUpdate struct {
	Stats []PupStats `json:"stats"`
}
