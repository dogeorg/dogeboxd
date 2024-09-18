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
	GetFastMonChannel() chan string
	GetFastStatChannel() chan map[string]ProcStatus
}

// actively listen for systemd journal entries
// for a given systemd service, close channel
// when done
type JournalReader interface {
	GetJournalChan(string) (context.CancelFunc, chan string, error)
}

type LogTailer interface {
	GetChan(string) (context.CancelFunc, chan string, error)
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

type SourceDetailsPup struct {
	Location string `json:"location"`
}

type SourceDetails struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Pups        []SourceDetailsPup `json:"pups"`
}

type SourceManager interface {
	GetAll(ignoreCache bool) (map[string]ManifestSourceList, error)
	GetSourceManifest(sourceId, pupName, pupVersion string) (pup.PupManifest, ManifestSource, error)
	GetSourcePup(sourceId, pupName, pupVersion string) (ManifestSourcePup, error)
	GetSource(name string) (ManifestSource, error)
	AddSource(location string) (ManifestSource, error)
	RemoveSource(id string) error
	DownloadPup(diskPath, sourceId, pupName, pupVersion string) error
	GetAllSourceConfigurations() []ManifestSourceConfiguration
}

type ManifestSourcePup struct {
	Name     string
	Location map[string]string
	Version  string
	Manifest pup.PupManifest
}

type ManifestSourceList struct {
	Config      ManifestSourceConfiguration
	LastChecked time.Time
	Pups        []ManifestSourcePup
}

type ManifestSource interface {
	ValidateFromLocation(location string) (ManifestSourceConfiguration, error)
	Config() ManifestSourceConfiguration
	List(ignoreCache bool) (ManifestSourceList, error)
	Download(diskPath string, remoteLocation map[string]string) error
}

type ManifestSourceConfiguration struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Type        string `json:"type"`
}

type NixPupContainerServiceValues struct {
	NAME string
	EXEC string
	CWD  string
	ENV  []struct {
		KEY string
		VAL string
	}
}

type NixPupContainerTemplateValues struct {
	DATA_DIR          string
	CONTAINER_LOG_DIR string
	PUP_ID            string
	PUP_ENABLED       bool
	INTERNAL_IP       string
	PUP_PORTS         []struct {
		PORT   int
		PUBLIC bool
	}
	STORAGE_PATH string
	PUP_PATH     string
	NIX_FILE     string
	SERVICES     []NixPupContainerServiceValues
}

type NixSystemContainerConfigTemplatePupRequiresInternet struct {
	PUP_ID string
	PUP_IP string
}

type NixSystemContainerConfigTemplatePupTcpConnectionOtherPup struct {
	NAME  string
	ID    string
	IP    string
	PORTS []struct {
		PORT int
	}
}

type NixSystemContainerConfigTemplatePupTcpConnection struct {
	NAME       string
	ID         string
	IP         string
	OTHER_PUPS []NixSystemContainerConfigTemplatePupTcpConnectionOtherPup
}

type NixSystemContainerConfigTemplateValues struct {
	DOGEBOX_HOST_IP         string
	DOGEBOX_CONTAINER_CIDR  string
	PUPS_REQUIRING_INTERNET []NixSystemContainerConfigTemplatePupRequiresInternet
	PUPS_TCP_CONNECTIONS    []NixSystemContainerConfigTemplatePupTcpConnection
}

type NixFirewallTemplateValues struct {
	SSH_ENABLED bool
	PUP_PORTS   []struct {
		PORT   int
		PUBLIC bool
		PUP_ID string
	}
}

type NixSystemTemplateValues struct {
	SYSTEM_HOSTNAME string
	SSH_ENABLED     bool
	SSH_KEYS        []string
}

type NixIncludesFileTemplateValues struct {
	PUP_IDS []string
}

type NixNetworkTemplateValues struct {
	USE_ETHERNET  bool
	USE_WIRELESS  bool
	INTERFACE     string
	WIFI_SSID     string
	WIFI_PASSWORD string
}

type NixManager interface {
	Rebuild() error
	RebuildBoot() error
	InitSystem() error
	UpdateIncludeFile(pups PupManager) error
	WriteDogeboxNixFile(filename string, content string) error
	WritePupFile(pupState PupState) error
	RemovePupFile(pupId string) error
	UpdateSystem(values NixSystemTemplateValues) error
	UpdateSystemContainerConfiguration() error
	UpdateNetwork(values NixNetworkTemplateValues) error
}
