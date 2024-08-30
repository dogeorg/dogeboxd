package dogeboxd

import (
	"context"
	"time"

	"github.com/dogeorg/dogeboxd/pkg/pup"
)

// see ./system/ for implementations

// handle jobs on behalf of Dogeboxd and
// return them via it's own update channel.
type SystemUpdater interface {
	AddJob(Job)
	GetUpdateChannel() chan Job
}

// monitors systemd services and returns stats
type SystemMonitor interface {
	GetMonChannel() chan []string
	GetStatChannel() chan map[string]ProcStatus
}

// actively listen for systemd journal entries
// for a given systemd service, close channel
// when done
type JournalReader interface {
	GetJournalChan(string) (context.CancelFunc, chan string, error)
}

// SystemMonitor issues these for monitored PUPs
type ProcStatus struct {
	CPUPercent float64
	MEMPercent float64
	MEMMb      float64
	Running    bool
}

type DogeboxStateInitialSetup struct {
	HasGeneratedKey    bool `json:"hasGeneratedKey"`
	HasSetNetwork      bool `json:"hasSetNetwork"`
	HasFullyConfigured bool `json:"hasFullyConfigured"`
}

type DogeboxState struct {
	InitialState DogeboxStateInitialSetup
}

type NetworkState struct {
	CurrentNetwork SelectedNetwork
	PendingNetwork SelectedNetwork
}

type SourceState struct {
	SourceConfigs []ManifestSourceConfiguration
}

type State struct {
	Network NetworkState
	Dogebox DogeboxState
	Sources SourceState
}

type StateManager interface {
	Get() State
	SetNetwork(s NetworkState)
	SetDogebox(s DogeboxState)
	SetSources(s SourceState)
	Save() error
	Load() error
}

type LifecycleManager interface {
	Shutdown()
	Reboot()
}

type NetworkManager interface {
	GetAvailableNetworks() []NetworkConnection
	SetPendingNetwork(selectedNetwork SelectedNetwork) error
	TryConnect() error
}

type NetworkConnection interface {
	networkConnectionMarker()
}

type NetworkEthernet struct {
	Type      string `json:"type"`
	Interface string `json:"interface"`
}

type NetworkWifi struct {
	Type      string            `json:"type"`
	Interface string            `json:"interface"`
	Ssids     []NetworkWifiSSID `json:"ssids"`
}

type NetworkWifiSSID struct {
	Ssid       string `json:"ssid"`
	Bssid      string `json:"bssid"` // TODO: this should probably not be a string?
	Encryption string `json:"encryption,omitempty"`
}

func (n NetworkEthernet) networkConnectionMarker() {}
func (n NetworkWifi) networkConnectionMarker()     {}

type SelectedNetwork interface {
	selectedNetworkMarker()
}

type SelectedNetworkEthernet struct {
	SelectedNetwork
	Interface string `json:"interface"`
}

type SelectedNetworkWifi struct {
	SelectedNetwork
	Interface  string `json:"interface"`
	Ssid       string `json:"ssid"`
	Password   string `json:"password"`
	Encryption string `json:"encryption"`
	IsHidden   bool   `json:"isHidden"`
}

func (sn SelectedNetworkEthernet) selectedNetworkMarker() {}
func (sn SelectedNetworkWifi) selectedNetworkMarker()     {}

type NetworkConnector interface {
	Connect(network SelectedNetwork) error
}

type NetworkPersistor interface {
	Persist(network SelectedNetwork) error
}

type SourceManager interface {
	GetAll() (map[string]ManifestSourceList, error)
	GetSourceManifest(sourceName, pupName, pupVersion string) (pup.PupManifest, error)
	GetSourcePup(sourceName, pupName, pupVersion string) (ManifestSourcePup, error)
	GetSource(name string) (ManifestSource, error)
	AddSource(source ManifestSourceConfiguration) (ManifestSource, error)
	RemoveSource(name string) error
	DownloadPup(diskPath, sourceName, pupName, pupVersion string) error
	GetAllSourceConfigurations() []ManifestSourceConfiguration
}

type ManifestSourcePup struct {
	Name     string
	Location string
	Version  string
	Manifest pup.PupManifest
}

type ManifestSourceList struct {
	LastUpdated time.Time
	Pups        []ManifestSourcePup
}

type ManifestSource interface {
	Name() string
	Config() ManifestSourceConfiguration
	Validate() (bool, error)
	List(ignoreCache bool) (ManifestSourceList, error)
	Download(diskPath string, remoteLocation string) error
}

type ManifestSourceConfiguration struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
}
