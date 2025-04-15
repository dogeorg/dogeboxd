package dogeboxd

import (
	"context"
	"net"
	"time"
)

// see ./system/ for implementations

// handle jobs on behalf of Dogeboxd and
// return them via it's own update channel.
type SystemUpdater interface {
	AddJob(Job)
	GetUpdateChannel() chan Job

	// These ideally should not be on here, but we currently don't
	// have a way to wait for a SystemUpdater event to finish.
	AddSSHKey(key string, l SubLogger) error
	EnableSSH(l SubLogger) error
	ListSSHKeys() ([]DogeboxStateSSHKey, error)
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

type DogeboxStateSSHKey struct {
	ID        string    `json:"id"`
	DateAdded time.Time `json:"dateAdded"`
	Key       string    `json:"key"`
}

type DogeboxStateSSHConfig struct {
	Enabled bool                 `json:"enabled"`
	Keys    []DogeboxStateSSHKey `json:"keys"`
}

type DogeboxState struct {
	InitialState  DogeboxStateInitialSetup
	Hostname      string
	KeyMap        string
	SSH           DogeboxStateSSHConfig
	StorageDevice string
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
	CloseDB() error
	OpenDB() error
	SetNetwork(s NetworkState) error
	SetDogebox(s DogeboxState) error
	SetSources(s SourceState) error
}

type LifecycleManager interface {
	Shutdown()
	Reboot()
}

type NetworkManager interface {
	GetAvailableNetworks() []NetworkConnection
	SetPendingNetwork(selectedNetwork SelectedNetwork, j Job) error
	TryConnect(nixPatch NixPatch) error
	GetLocalIP() (net.IP, error)
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
	Quality    float32 `json:"quality"`
	Signal     string `json:"signal"`
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
	Persist(nixPatch NixPatch, network SelectedNetwork)
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
	GetSourceManifest(sourceId, pupName, pupVersion string) (PupManifest, ManifestSource, error)
	GetSourcePup(sourceId, pupName, pupVersion string) (ManifestSourcePup, error)
	GetSource(name string) (ManifestSource, error)
	AddSource(location string) (ManifestSource, error)
	RemoveSource(id string) error
	DownloadPup(diskPath, sourceId, pupName, pupVersion string) error
	GetAllSourceConfigurations() []ManifestSourceConfiguration
}

type ManifestSourcePup struct {
	Name       string
	Location   map[string]string
	Version    string
	Manifest   PupManifest
	LogoBase64 string
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

type EnvEntry struct {
	KEY string
	VAL string
}

type NixPupContainerServiceValues struct {
	NAME string
	EXEC string
	CWD  string
	ENV  []EnvEntry
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
	PUP_ENV      []EnvEntry
	GLOBAL_ENV   []EnvEntry
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
	KEYMAP          string
	SSH_ENABLED     bool
	SSH_KEYS        []DogeboxStateSSHKey
}

type NixIncludesFileTemplateValues struct {
	NIX_DIR string
	PUP_IDS []string
}

type NixNetworkTemplateValues struct {
	USE_ETHERNET  bool
	USE_WIRELESS  bool
	INTERFACE     string
	WIFI_SSID     string
	WIFI_PASSWORD string
}

type NixStorageOverlayTemplateValues struct {
	STORAGE_DEVICE string
	DATA_DIR       string
	DBX_UID        string
}

type NixPatchApplyOptions struct {
	RebuildBoot        bool
	DangerousNoRebuild bool
}

type NixPatch interface {
	State() string
	Apply() error
	ApplyCustom(options NixPatchApplyOptions) error

	Cancel() error

	UpdateSystemContainerConfiguration(values NixSystemContainerConfigTemplateValues)
	UpdateFirewall(values NixFirewallTemplateValues)
	UpdateSystem(values NixSystemTemplateValues)
	UpdateNetwork(values NixNetworkTemplateValues)
	UpdateIncludesFile(values NixIncludesFileTemplateValues)
	WritePupFile(pupId string, values NixPupContainerTemplateValues)
	RemovePupFile(pupId string)
	UpdateStorageOverlay(values NixStorageOverlayTemplateValues)
}

type NixManager interface {
	// NixPatch passthrough helpers.
	InitSystem(patch NixPatch, dbxState DogeboxState)
	UpdateIncludesFile(patch NixPatch, pups PupManager)
	WritePupFile(patch NixPatch, state PupState, dbxState DogeboxState)
	RemovePupFile(patch NixPatch, pupId string)
	UpdateSystemContainerConfiguration(patch NixPatch)
	UpdateFirewallRules(patch NixPatch, dbxState DogeboxState)
	UpdateNetwork(patch NixPatch, values NixNetworkTemplateValues)
	UpdateSystem(patch NixPatch, values NixSystemTemplateValues)
	UpdateStorageOverlay(patch NixPatch, partitionName string)

	RebuildBoot(log SubLogger) error
	Rebuild(log SubLogger) error

	NewPatch(log SubLogger) NixPatch
}

type SystemDiskSuitabilityEntry struct {
	Usable bool `json:"usable"`
	SizeOK bool `json:"sizeOK"`
}

type SystemDiskSuitability struct {
	Install       SystemDiskSuitabilityEntry `json:"install"`
	Storage       SystemDiskSuitabilityEntry `json:"storage"`
	IsAlreadyUsed bool                       `json:"isAlreadyUsed"`
}

type SystemDisk struct {
	Name        string                `json:"name"`
	Size        int64                 `json:"size"`
	SizePretty  string                `json:"sizePretty"`
	Suitability SystemDiskSuitability `json:"suitability"`
	BootMedia   bool                  `json:"bootMedia"`
	Label       string                `json:"label"`
	Path        string                `json:"path"`
	Children    []SystemDisk          `json:"children,omitempty"`
}

type BootstrapInstallationMode string

const (
	BootstrapInstallationModeCanInstall    BootstrapInstallationMode = "canInstall"
	BootstrapInstallationModeMustInstall   BootstrapInstallationMode = "mustInstall"
	BootstrapInstallationModeCannotInstall BootstrapInstallationMode = "cannotInstall"
)
