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
	ID string `json:"id"`
	// Seq is a monotonically increasing sequence number for ordering changes on the client.
	// It is assigned server-side when the Change is emitted.
	Seq uint64 `json:"seq"`
	// TS is the server timestamp in milliseconds since epoch, assigned when emitted.
	TS     int64  `json:"ts"`
	Error  string `json:"error"`
	Type   string `json:"type"`
	Update Update `json:"update"`
}

// Represents some information about an action underway
type ActionProgress struct {
	ActionID  string        `json:"actionID"`
	PupID     string        `json:"pupID"`      // optional, only if a pup action
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
 *
 * All Actions must implement ActionName() to provide
 * a string identifier for the action type.
 */
type Action interface {
	ActionName() string
}

// Install a pup on the system
type InstallPup struct {
	PupName    string
	PupVersion string
	SourceId   string
	Options    AdoptPupOptions

	SessionToken string
}

func (InstallPup) ActionName() string { return "install" }

// InstallPups represents a batch installation of multiple pups
type InstallPups []InstallPup

func (InstallPups) ActionName() string { return "install" }

// Uninstalling a pup will remove container
// configuration, but keep storage.
type UninstallPup struct {
	PupID string
}

func (UninstallPup) ActionName() string { return "uninstall" }

// Purging a pup will remove the container storage.
type PurgePup struct {
	PupID string
}

func (PurgePup) ActionName() string { return "purge" }

// Enable a previously disabled pup
type EnablePup struct {
	PupID string
}

func (EnablePup) ActionName() string { return "enable" }

// Disable (stop) a running pup
type DisablePup struct {
	PupID string
}

func (DisablePup) ActionName() string { return "disable" }

// UpgradePup upgrades a pup to a new version while preserving config and data
type UpgradePup struct {
	PupID         string
	TargetVersion string
	SourceId      string // Source to download new version from
}

func (UpgradePup) ActionName() string { return "upgrade" }

// RollbackPupUpgrade rolls back a pup to its previous version after a failed upgrade
type RollbackPupUpgrade struct {
	PupID string
}

func (RollbackPupUpgrade) ActionName() string { return "rollback" }

// Updates the config values in a PUPState object
type UpdatePupConfig struct {
	PupID   string
	Payload map[string]string
}

func (UpdatePupConfig) ActionName() string { return "config" }

// Updates the providers of dependant interfaces for this pup
type UpdatePupProviders struct {
	PupID   string
	Payload map[string]string
}

func (UpdatePupProviders) ActionName() string { return "providers" }

// Updates hooks for this pup
type UpdatePupHooks struct {
	PupID   string
	Payload []PupHook
}

func (UpdatePupHooks) ActionName() string { return "hooks" }

// updates the custom metrics for a pup
type UpdateMetrics struct {
	PupID   string
	Payload map[string]PupMetric
}

func (UpdateMetrics) ActionName() string { return "metrics" }

type PupMetric struct {
	Value any `json:"value"`
}

type UpdatePendingSystemNetwork struct {
	Network SelectedNetwork
}

func (UpdatePendingSystemNetwork) ActionName() string { return "network" }

type InitialBootstrap struct {
	ReflectorToken              string
	ReflectorHost               string
	InitialSSHKey               string
	UseFoundationOSBinaryCache  bool
	UseFoundationPupBinaryCache bool
}

func (InitialBootstrap) ActionName() string { return "initial-bootstrap" }

type (
	EnableSSH  struct{}
	DisableSSH struct{}
)

func (EnableSSH) ActionName() string  { return "enable-ssh" }
func (DisableSSH) ActionName() string { return "disable-ssh" }

type AddSSHKey struct {
	Key string
}

func (AddSSHKey) ActionName() string { return "add-ssh-key" }

type RemoveSSHKey struct {
	ID string
}

func (RemoveSSHKey) ActionName() string { return "remove-ssh-key" }

type SaveCustomNix struct {
	Content string `json:"content"`
}

func (SaveCustomNix) ActionName() string { return "save-custom-nix" }

// Import blockchain data to the system (not tied to a specific pup)
type ImportBlockchainData struct{}

func (ImportBlockchainData) ActionName() string { return "import-blockchain" }

type UpdateTimezone struct {
	Timezone string
}

func (UpdateTimezone) ActionName() string { return "update-timezone" }

type UpdateKeymap struct {
	Keymap string
}

func (UpdateKeymap) ActionName() string { return "update-keymap" }

type SystemUpdate struct {
	Package string
	Version string
}

func (SystemUpdate) ActionName() string { return "system-update" }

type UpdateNixCache struct {
}

func (UpdateNixCache) ActionName() string { return "update-nix-cache" }

type AddBinaryCache struct {
	Host string
	Key  string
}

func (AddBinaryCache) ActionName() string { return "add-binary-cache" }

type RemoveBinaryCache struct {
	ID string
}

func (RemoveBinaryCache) ActionName() string { return "remove-binary-cache" }

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
