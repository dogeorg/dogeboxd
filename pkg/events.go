package dogeboxd

import "time"

// A Job is created when an Action is recieved by the system.
// Jobs are passed through the Dogeboxd service and result in
// a Change being send to the client via websockets.
type Job struct {
	A       Action
	ID      string
	Err     string
	Success any
	Start   time.Time // set when the job is first created, for calculating duration
	Logger  *actionLogger
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

// Represents some information about an action underway
type ActionProgress struct {
	ActionID  string        `json:"actionID"`
	PupID     string        `json:pupID`        // optional, only if a pup action
	Progress  int           `json:"progress"`   // 0-100
	Step      string        `json:"step"`       // a unique name for the step we're up to, ie: installing
	Msg       string        `json:"msg"`        // the message line
	Error     bool          `json:"error"`      // if this represents an error or not
	StepTaken time.Duration `json:"step_taken"` // time taken from previous step
}

/* Actions are passed to the dogeboxd service via its
 * AddAction method, and represent tasks that need to
 * be done such as installing a package, starting or
 * stopping a service etc.
 */
type Action any

// Install a pup on the system
type InstallPup struct {
	PupName      string
	PupVersion   string
	SourceId     string
	SessionToken string
}

// Uninstalling a pup will remove container
// configuration, but keep storage.
type UninstallPup struct {
	PupID string
}

// Purging a pup will remove the container storage.
type PurgePup struct {
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

// Updates the config values in a PUPState object
type UpdatePupConfig struct {
	PupID   string
	Payload map[string]string
}

// Updates the providers of dependant interfaces for this pup
type UpdatePupProviders struct {
	PupID   string
	Payload map[string]string
}

// Updates hooks for this pup
type UpdatePupHooks struct {
	PupID   string
	Payload []PupHook
}

// updates the custom metrics for a pup
type UpdateMetrics struct {
	PupID   string
	Payload map[string]PupMetric
}

type PupMetric struct {
	Value any `json:"value"`
}

type UpdatePendingSystemNetwork struct {
	Network SelectedNetwork
}

type (
	EnableSSH  struct{}
	DisableSSH struct{}
)

type AddSSHKey struct {
	Key string
}

type RemoveSSHKey struct {
	ID string
}

type SystemUpdateRequest struct {
	Package string
	Version string
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
