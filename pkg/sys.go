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

type RepositoryState struct {
	Repositories []ManifestRepository
}

type State struct {
	Network    NetworkState
	Dogebox    DogeboxState
	Repository RepositoryState
}

type StateManager interface {
	Get() State
	SetNetwork(s NetworkState)
	SetDogebox(s DogeboxState)
	SetRepository(s RepositoryState)
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

type RepositoryManager interface {
	GetAll() (map[string]ManifestRepositoryList, error)
	GetRepositories() []ManifestRepository
	AddRepository(repo ManifestRepositoryConfiguration) (ManifestRepository, error)
	RemoveRepository(name string) error
}

type ManifestRepositoryPup struct {
	Name     string
	Location string
	Version  string
	Manifest pup.PupManifest
}

type ManifestRepositoryList struct {
	LastUpdated time.Time
	Pups        []ManifestRepositoryPup
}

type ManifestRepository interface {
	Name() string
	Config() ManifestRepositoryConfiguration
	Validate() (bool, error)
	List(ignoreCache bool) (ManifestRepositoryList, error)
	Download(diskPath string, remoteLocation string) error
}

type ManifestRepositoryConfiguration struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
}
