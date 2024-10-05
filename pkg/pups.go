package dogeboxd

import (
	"encoding/json"

	"github.com/dogeorg/dogeboxd/pkg/pup"
)

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
	Manifest     pup.PupManifest             `json:"manifest"`
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
	Interface             string                            `json:"interface"`
	Version               string                            `json:"version"`
	CurrentProvider       string                            `json:"currentProvider"`
	InstalledProviders    []string                          `json:"installedProviders"`
	InstallableProviders  []pup.PupManifestDependencySource `json:"InstallableProviders"`
	DefaultSourceProvider pup.PupManifestDependencySource   `json:"DefaultProvider"`
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
