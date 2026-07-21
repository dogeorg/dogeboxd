package dogeboxd

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type InstallationState string

type PupRunStatus string

// Pup installation states
const (
	STATE_INSTALLING   InstallationState = "installing"
	STATE_UPGRADING    InstallationState = "upgrading"
	STATE_READY        InstallationState = "ready"
	STATE_UNREADY      InstallationState = "unready"
	STATE_UNINSTALLING InstallationState = "uninstalling"
	STATE_UNINSTALLED  InstallationState = "uninstalled"
	STATE_PURGING      InstallationState = "purging"
	STATE_BROKEN       InstallationState = "broken"
)

// Pup runtime statuses
const (
	STATE_STOPPED  PupRunStatus = "stopped"
	STATE_STARTING PupRunStatus = "starting"
	STATE_RUNNING  PupRunStatus = "running"
	STATE_STOPPING PupRunStatus = "stopping"
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
	PUP_PURGED                   = iota
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
	ConfigSaved  bool                        `json:"configSaved"`  // Has config been saved at least once?
	Providers    map[string]string           `json:"providers"`    // providers of interface dependencies
	Hooks        []PupHook                   `json:"hooks"`        // webhooks
	Installation InstallationState           `json:"installation"` // see table above and constants
	BrokenReason string                      `json:"brokenReason"` // reason for being in a broken state
	Enabled      bool                        `json:"enabled"`      // Is this pup supposed to be running?
	NeedsConf    bool                        `json:"needsConf"`    // Has all required config been provided?
	NeedsDeps    bool                        `json:"needsDeps"`    // Have all dependencies been met?
	IP           string                      `json:"ip"`           // Internal IP for this pup
	Version      string                      `json:"version"`
	WebUIs       []PupWebUI                  `json:"webUIs"`

	IsDevModeEnabled bool     `json:"isDevModeEnabled"`
	DevModeServices  []string `json:"devModeServices"`

	// Update management
	SkippedVersion string `json:"skippedVersion,omitempty"` // Version up to which updates are skipped
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
	Status        PupRunStatus      `json:"status"`
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
	Optional              bool                          `json:"optional"`
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

type AdoptPupOptions struct {
	/// Install pup with development features enabled
	DevMode bool
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
	AdoptPup(m PupManifest, source ManifestSource, options AdoptPupOptions) (string, error)

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

	// GetPupHealthState returns the health state report for a pup.
	GetPupHealthState(pup *PupState) PupHealthStateReport

	// CalculateDeps calculates the dependencies for a pup.
	CalculateDeps(pupID string) ([]PupDependencyReport, error)

	// SetSourceManager sets the SourceManager for the PupManager.
	SetSourceManager(sourceManager SourceManager)

	// FastPollPup initiates a rapid polling of a specific pup for debugging or immediate updates.
	FastPollPup(pupId string)

	GetPupSpecificEnvironmentVariablesForContainer(pupID string) map[string]string

	// StopPup stops a running pup by disabling it and triggering a rebuild
	StopPup(pupID string, nixManager NixManager, logger SubLogger) error

	// StartPup starts a stopped pup by enabling it and triggering a rebuild
	StartPup(pupID string, nixManager NixManager, logger SubLogger) error

	// Snapshot management
	CreateSnapshot(pupState PupState) error
	GetSnapshot(pupID string) (*PupVersionSnapshot, error)
	HasSnapshot(pupID string) bool
	DeleteSnapshot(pupID string) error
	ListSnapshots() ([]string, error)
	CleanOldSnapshots(maxAge time.Duration) (int, error)

	// ClearCacheEntry removes a specific pup from the update cache
	ClearCacheEntry(pupID string)
}

func SetPupInstallation(state InstallationState) func(*PupState, *[]Pupdate) {
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
		if p.Config == nil {
			p.Config = map[string]string{}
		}

		fieldIndex := ManifestConfigFieldIndex(p.Manifest.Config)

		for k, v := range newFields {
			if _, ok := fieldIndex[k]; !ok {
				continue
			}
			p.Config[k] = v
		}

		// Mark config as saved (satisfies showOnInstall requirement)
		p.ConfigSaved = true

		p.NeedsConf = ManifestConfigNeedsValues(p.Manifest.Config, p.Config)
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

func SetPupVersion(version string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Version = version
	}
}

func SetPupSkippedVersion(version string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.SkippedVersion = version
	}
}

func SetPupManifest(manifest PupManifest) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Manifest = manifest
		// Recalculate if config needs values based on new manifest
		p.NeedsConf = ManifestConfigNeedsValues(p.Manifest.Config, p.Config)
	}
}

func PupEnabled(b bool) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Enabled = b
		// Send a pupdate so frontend is notified of enabled state changes
		// (important for upgrade operations where pup is re-enabled after upgrade)
		*pu = append(*pu, Pupdate{
			ID:    p.ID,
			Event: PUP_CHANGED_INSTALLATION,
			State: *p,
		})
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

// ===========================================
// =============== Pup Updates ===============
// ===========================================

/* Pup update types
 */
// PupUpdateInfo tracks available updates for a pup
type PupUpdateInfo struct {
	PupID             string       `json:"pupId"`
	CurrentVersion    string       `json:"currentVersion"`
	LatestVersion     string       `json:"latestVersion"`
	AvailableVersions []PupVersion `json:"availableVersions"`
	UpdateAvailable   bool         `json:"updateAvailable"`
	LastChecked       time.Time    `json:"lastChecked"`
}

// PupVersion represents a version available for update
type PupVersion struct {
	Version          string                `json:"version"`
	ReleaseNotes     string                `json:"releaseNotes,omitempty"`
	ReleaseDate      *time.Time            `json:"releaseDate,omitempty"`
	ReleaseURL       string                `json:"releaseUrl,omitempty"`
	BreakingChanges  []string              `json:"breakingChanges,omitempty"`
	InterfaceChanges []PupInterfaceVersion `json:"interfaceChanges,omitempty"`
}

// PupInterfaceVersion tracks changes to provided interfaces
type PupInterfaceVersion struct {
	InterfaceName string   `json:"interfaceName"`
	OldVersion    string   `json:"oldVersion"`
	NewVersion    string   `json:"newVersion"`
	ChangeType    string   `json:"changeType"`   // "major", "minor", "patch"
	AffectedPups  []string `json:"affectedPups"` // PupIDs that depend on this interface
}

// PupUpdatePreviousVersion tracks update history for rollback
type PupUpdatePreviousVersion struct {
	PupID           string              `json:"pupId"`
	PreviousVersion *PupVersionSnapshot `json:"previousVersion"` // Only keep last version
}

// PupVersionSnapshot stores data needed for rollback
// Note: User data in storage directory is NOT snapshotted - only state/config
type PupVersionSnapshot struct {
	Version        string            `json:"version"`
	Manifest       PupManifest       `json:"manifest"`
	Config         map[string]string `json:"config"`
	Providers      map[string]string `json:"providers"`
	Enabled        bool              `json:"enabled"`
	SnapshotDate   time.Time         `json:"snapshotDate"`
	SourceID       string            `json:"sourceId"`
	SourceLocation string            `json:"sourceLocation"` // For re-downloading
}

/* Pup update actions
 */
type CheckPupUpdates struct {
	PupID string // Empty string = check all pups
}

func (CheckPupUpdates) ActionName() string { return "check-updates" }

// PupUpdatesCheckedEvent is emitted when a pup update check completes
type PupUpdatesCheckedEvent struct {
	PupsChecked      int  `json:"pupsChecked"`
	UpdatesAvailable int  `json:"updatesAvailable"`
	IsPeriodicCheck  bool `json:"isPeriodicCheck"`
}

/* The PupUpdateChecker is used to check for pup updates
 */
type PupUpdateChecker interface {
	// CheckForUpdates checks for updates for a specific pup
	CheckForUpdates(pupID string) (PupUpdateInfo, error)

	// CheckAllPupUpdates checks for updates for all installed pups
	CheckAllPupUpdates() map[string]PupUpdateInfo

	// GetCachedUpdateInfo retrieves update information from the cache
	GetCachedUpdateInfo(pupID string) (PupUpdateInfo, bool)

	// GetAllCachedUpdates retrieves all update information from the cache
	GetAllCachedUpdates() map[string]PupUpdateInfo

	// ClearCacheEntry removes a specific pup from the update cache
	ClearCacheEntry(pupID string)

	// StartPeriodicCheck starts a background goroutine that checks for updates periodically
	StartPeriodicCheck(stop chan bool)

	// GetEventChannel returns the channel for update check completion events
	GetEventChannel() <-chan PupUpdatesCheckedEvent

	// DetectInterfaceChanges compares interfaces between two manifests and returns changes
	DetectInterfaceChanges(oldManifest, newManifest PupManifest) []PupInterfaceVersion
}
