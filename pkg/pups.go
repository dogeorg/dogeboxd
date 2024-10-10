package dogeboxd

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
)

// Pup states
const (
	STATE_INSTALLING   string = "installing"
	STATE_READY        string = "ready"
	STATE_UNREADY      string = "unready"
	STATE_UNINSTALLING string = "uninstalling"
	STATE_UNINSTALLED  string = "uninstalled"
	STATE_PURGING      string = "purging"
	STATE_BROKEN       string = "broken"
	STATE_STOPPED      string = "stopped"
	STATE_STARTING     string = "starting"
	STATE_RUNNING      string = "running"
	STATE_STOPPING     string = "stopping"
)

// Pup broken reasons
const (
	BROKEN_REASON_STATE_UPDATE_FAILED          string = "state_update_failed"
	BROKEN_REASON_DOWNLOAD_FAILED              string = "download_failed"
	BROKEN_REASON_NIX_FILE_MISSING             string = "nix_file_missing"
	BROKEN_REASON_NIX_HASH_MISMATCH            string = "nix_hash_mismatch"
	BROKEN_REASON_STORAGE_CREATION_FAILED      string = "storage_creation_failed"
	BROKEN_REASON_DELEGATE_KEY_CREATION_FAILED string = "delegate_key_creation_failed"
	BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED    string = "delegate_key_write_failed"
	BROKEN_REASON_ENABLE_FAILED                string = "enable_failed"
	BROKEN_REASON_NIX_APPLY_FAILED             string = "nix_apply_failed"
)

const (
	PUP_CHANGED_INSTALLATION int = iota
	PUP_ADOPTED                  = iota
)

// PupManager Errors
var (
	ErrPupNotFound      = errors.New("pup not found")
	ErrPupAlreadyExists = errors.New("pup already exists")
)

/* Pup state vs pup stats
 * ┌─────────────────────────────┬───────────────────────────────┐
 * │PupState.Installation        │ PupStats.Status               │
 * ├─────────────────────────────┼───────────────────────────────┤
 * │                             │                               │
 * │installing                   │    stopped                    │
 * │ready                       ─┼─>  starting                   │
 * │unready                      │    running                    │
 * │uninstalling                 │    stopping                   │
 * │uninstalled                  │                               │
 * │broken                       │                               │
 * └─────────────────────────────┴───────────────────────────────┘
 *
 * Valid actions: install, stop, start, restart, uninstall
 */

// PupState is persisted to disk
type PupState struct {
	ID           string                      `json:"id"`
	LogoBase64   string                      `json:"logoBase64"`
	Source       ManifestSourceConfiguration `json:"source"`
	Manifest     PupManifest                 `json:"manifest"`
	Config       map[string]string           `json:"config"`
	Providers    map[string]string           `json:"providers"`    // providers of interface dependencies
	Hooks        []PupHook                   `json:"hooks"`        // webhooks
	Installation string                      `json:"installation"` // see table above and constants
	BrokenReason string                      `json:"brokenReason"` // reason for being in a broken state
	Enabled      bool                        `json:"enabled"`      // Is this pup supposed to be running?
	NeedsConf    bool                        `json:"needsConf"`    // Has all required config been provided?
	NeedsDeps    bool                        `json:"needsDeps"`    // Have all dependencies been met?
	IP           string                      `json:"ip"`           // Internal IP for this pup
	Version      string                      `json:"version"`
	WebUIs       []PupWebUI                  `json:"webUIs"`
}

// Represents a Web UI exposed port from the manifest
type PupWebUI struct {
	Name     string `json:"name"`
	Internal int    `json:"-"`
	Port     int    `json:"port"`
}

type PupHook struct {
	Port int    `json:"port"`
	Path string `json:"path"`
	ID   string `json:"id"`
}

type PupMetrics[T any] struct {
	Name   string     `json:"name"`
	Label  string     `json:"label"`
	Type   string     `json:"type"`
	Values *Buffer[T] `json:"values"`
}

// PupStats is not persisted to disk, and holds the running
// stats for the pup process, ie: disk, CPU, etc.
type PupStats struct {
	ID            string            `json:"id"`
	Status        string            `json:"status"`
	SystemMetrics []PupMetrics[any] `json:"systemMetrics"`
	Metrics       []PupMetrics[any] `json:"metrics"`
	Issues        PupIssues         `json:"issues"`
}

type PupLogos struct {
	MainLogoBase64 string `json:"mainLogoBase64"`
}

type PupAsset struct {
	Logos PupLogos `json:"logos"`
}

type PupIssues struct {
	DepsNotRunning   []string `json:"depsNotRunning"`
	HealthWarnings   []string `json:"healthWarnings"`
	UpgradeAvaialble bool     `json:"upgradeAvailable"`
}

type PupDependencyReport struct {
	Interface             string                        `json:"interface"`
	Version               string                        `json:"version"`
	CurrentProvider       string                        `json:"currentProvider"`
	InstalledProviders    []string                      `json:"installedProviders"`
	InstallableProviders  []PupManifestDependencySource `json:"InstallableProviders"`
	DefaultSourceProvider PupManifestDependencySource   `json:"DefaultProvider"`
}

type PupHealthStateReport struct {
	Issues    PupIssues
	NeedsConf bool
	NeedsDeps bool
}

// Represents a change to pup state
type Pupdate struct {
	ID    string
	Event int // see consts above ^
	State PupState
}

type Buffer[T any] struct {
	Values []T
	Tail   int
}

func NewBuffer[T any](size int) *Buffer[T] {
	return &Buffer[T]{
		Values: make([]T, size),
		Tail:   0,
	}
}

func (b *Buffer[T]) Add(value T) {
	b.Values[b.Tail] = value
	b.Tail = (b.Tail + 1) % len(b.Values)
}

func (b *Buffer[T]) GetValues() []T {
	firstN := make([]T, len(b.Values))
	if b.Tail > 0 {
		copy(firstN, b.Values[b.Tail:])
		copy(firstN[len(b.Values)-b.Tail:], b.Values[:b.Tail])
	} else {
		copy(firstN, b.Values)
	}
	return firstN
}

func (b *Buffer[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.GetValues())
}

/* The PupManager is responsible for all aspects of the pup lifecycle
 * see pkg/pup/manager.go
 */
type PupManager interface {
	// Run starts the PupManager as a service.
	Run(started, stopped chan bool, stop chan context.Context) error

	// GetUpdateChannel returns a channel for receiving pup updates.
	GetUpdateChannel() chan Pupdate

	// GetStatsChannel returns a channel for receiving pup stats.
	GetStatsChannel() chan []PupStats

	// GetStateMap returns a map of all pup states.
	GetStateMap() map[string]PupState

	// GetStatsMap returns a map of all pup stats.
	GetStatsMap() map[string]PupStats

	// GetAssetsMap returns a map of pup assets like logos.
	GetAssetsMap() map[string]PupAsset

	// AdoptPup adds a new pup from a manifest. It returns the PupID and an error if any.
	AdoptPup(m PupManifest, source ManifestSource) (string, error)

	// UpdatePup updates the state of a pup with provided update functions.
	UpdatePup(id string, updates ...func(*PupState, *[]Pupdate)) (PupState, error)

	// PurgePup removes a pup and its state from the manager.
	PurgePup(pupId string) error

	// GetPup retrieves the state and stats for a specific pup by ID.
	GetPup(id string) (PupState, PupStats, error)

	// FindPupByIP retrieves a pup by its assigned IP address.
	FindPupByIP(ip string) (PupState, PupStats, error)

	// GetAllFromSource retrieves all pups from a specific source.
	GetAllFromSource(source ManifestSourceConfiguration) []*PupState

	// GetPupFromSource retrieves a specific pup by name from a source.
	GetPupFromSource(name string, source ManifestSourceConfiguration) *PupState

	// GetMetrics retrieves the metrics for a specific pup.
	GetMetrics(pupId string) map[string]interface{}

	// UpdateMetrics updates the metrics for a pup based on provided data.
	UpdateMetrics(u UpdateMetrics)

	// CanPupStart checks if a pup can start based on its current state and dependencies.
	CanPupStart(pupId string) (bool, error)

	// CalculateDeps calculates the dependencies for a pup.
	CalculateDeps(pupID string) ([]PupDependencyReport, error)

	// SetSourceManager sets the SourceManager for the PupManager.
	SetSourceManager(sourceManager SourceManager)

	// FastPollPup initiates a rapid polling of a specific pup for debugging or immediate updates.
	FastPollPup(pupId string)

	GetPupSpecificEnvironmentVariablesForContainer(pupID string) map[string]string
}

/*****************************************************************************/
/*                Varadic Update Funcs for PupManager.UpdatePup:             */
/*****************************************************************************/

func SetPupInstallation(state string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Installation = state
		*pu = append(*pu, Pupdate{
			ID:    p.ID,
			Event: PUP_CHANGED_INSTALLATION,
			State: *p,
		})
	}
}

func SetPupBrokenReason(reason string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.BrokenReason = reason
	}
}

func SetPupConfig(newFields map[string]string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		for k, v := range newFields {
			p.Config[k] = v
		}
	}
}

func SetPupProviders(newProviders map[string]string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		if p.Providers == nil {
			p.Providers = make(map[string]string)
		}

		for k, v := range newProviders {
			p.Providers[k] = v
		}
	}
}

func PupEnabled(b bool) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Enabled = b
	}
}

func SetPupHooks(newHooks []PupHook) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		if p.Hooks == nil {
			p.Hooks = []PupHook{}
		}

		for _, hook := range newHooks {
			id, err := newID(16)
			if err != nil {
				fmt.Println("couldn't generate random ID for hook")
				continue
			}
			hook.ID = id
			p.Hooks = append(p.Hooks, hook)
		}
	}
}

// Generate a somewhat random ID string
func newID(l int) (string, error) {
	var ID string
	b := make([]byte, l)
	_, err := rand.Read(b)
	if err != nil {
		return ID, err
	}
	return fmt.Sprintf("%x", b), nil
}
