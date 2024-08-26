package dogeboxd

import (
	"encoding/json"
)

const (
	STATE_INSTALLING   string = "installing"
	STATE_READY               = "ready"
	STATE_UNREADY             = "unready"
	STATE_UNINSTALLING        = "uninstalling"
	STATE_UNINSTALLED         = "uninstalled"
	STATE_BROKEN              = "broken"
	STATE_STOPPED             = "stopped"
	STATE_STARTING            = "starting"
	STATE_RUNNING             = "running"
	STATE_STOPPING            = "stopping"
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
	ID           string            `json:"id"`
	Manifest     PupManifest       `json:"manifest"`
	Config       map[string]string `json:"config"`
	Installation string            `json:"installation"`
	Enabled      bool              `json:"enabled"`
	NeedsConf    bool              `json:"needsConf"`
	NeedsDeps    bool              `json:"needsDeps"`
	Version      string            `json:"enabled"`
}

// PupStats is not persisted to disk, and holds the running
// stats for the pup process, ie: disk, CPU, etc.
type PupStats struct {
	ID       string      `json:"id"`
	Status   string      `json:"status"`
	StatCPU  FloatBuffer `json:"status_cpu"`
	StatMEM  FloatBuffer `json:"status_mem"`
	StatDISK FloatBuffer `json:"status_disk"`
}

type FloatBuffer struct {
	Values []float32
	Head   int
}

func NewFloatBuffer(size int) FloatBuffer {
	return FloatBuffer{
		Values: make([]float32, size),
		Head:   0,
	}
}

func (b *FloatBuffer) Add(value float32) {
	b.Values[b.Head] = value
	b.Head = (b.Head + 1) % len(b.Values)
}

func (b *FloatBuffer) GetValues() []float32 {
	lastN := make([]float32, len(b.Values))
	for i := 0; i < len(b.Values); i++ {
		lastN[i] = b.Values[(b.Head-i-1+len(b.Values))%len(b.Values)]
	}
	return lastN
}

func (b *FloatBuffer) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.GetValues())
}
