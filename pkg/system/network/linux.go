package network

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	network_connector "github.com/dogeorg/dogeboxd/pkg/system/network/connector"
	network_persistor "github.com/dogeorg/dogeboxd/pkg/system/network/persistor"
	network_wifi "github.com/dogeorg/dogeboxd/pkg/system/network/wifi"
	"github.com/mdlayher/wifi"
)

var _ dogeboxd.NetworkManager = &NetworkManagerLinux{}

type NetworkManagerLinux struct {
	dogeboxd.NetworkManager

	sm      dogeboxd.StateManager
	scanner network_wifi.WifiScanner
	nix     dogeboxd.NixManager
}

type WiFiNetwork struct {
	SSID      string
	Address   string
	Signal    string
	Channel   string
	Frequency string
}

func (t NetworkManagerLinux) GetAvailableNetworks() []dogeboxd.NetworkConnection {
	availableNetworkConnections := []dogeboxd.NetworkConnection{}
	wifiInterfaceNames := []string{}

	wifiClient, err := wifi.New()
	if err != nil {
		log.Println("Could not init a wifi interface client, skipping:", err)
	} else {
		defer wifiClient.Close()

		wifiInterfaces, err := wifiClient.Interfaces()
		if err != nil {
			log.Println("Could not list wifi interfaces:", err)
		}

		for _, wifiInterface := range wifiInterfaces {
			ssids, err := t.scanner.Scan(wifiInterface.Name)
			if err != nil {
				log.Printf("Failed to scan for Wifi networks on %s: %s", wifiInterface.Name, err)
				continue
			}

			foundNetworks := []dogeboxd.NetworkWifiSSID{}

			for _, n := range ssids {
				// Ignore anything without an SSID
				if n.SSID == "" {
					continue
				}

				foundNetworks = append(foundNetworks, dogeboxd.NetworkWifiSSID{
					Ssid:       n.SSID,
					Bssid:      n.BSSID,
					Encryption: n.Encryption,
				})
			}

			availableNetworkConnections = append(availableNetworkConnections, dogeboxd.NetworkWifi{
				Type:      "wifi",
				Interface: wifiInterface.Name,
				Ssids:     foundNetworks,
			})
			wifiInterfaceNames = append(wifiInterfaceNames, wifiInterface.Name)
		}
	}

	allInterfaces, err := net.Interfaces()

	if err != nil {
		log.Printf("Failed to fetch system interfaces: %s", err)
		return availableNetworkConnections
	}

outer:
	for _, systemInterface := range allInterfaces {
		// Ignore anything that doesn't have a hardware address.
		if systemInterface.HardwareAddr == nil {
			continue
		}

		// Ignore if it starts with "ve-pup-" as
		// this is an internal pup-only interface.
		if strings.HasPrefix(systemInterface.Name, "ve-pup-") {
			continue
		}

		// If we've seen this as a wifi network, ignore it.
		for _, v := range wifiInterfaceNames {
			if v == systemInterface.Name {
				continue outer
			}
		}

		availableNetworkConnections = append(availableNetworkConnections, dogeboxd.NetworkEthernet{
			Type:      "ethernet",
			Interface: systemInterface.Name,
		})
	}

	return availableNetworkConnections
}

func (t NetworkManagerLinux) SetPendingNetwork(selectedNetwork dogeboxd.SelectedNetwork) error {
	var selectedIface string

	switch network := selectedNetwork.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		{
			log.Printf("Setting Ethernet network on interface: %s", network.Interface)
			selectedIface = network.Interface
		}

	case dogeboxd.SelectedNetworkWifi:
		{
			log.Printf("Setting WiFi network on interface: %s", network.Interface)
			log.Printf("SSIDs: %s, password: %s, encryption: %s", network.Ssid, network.Password, network.Encryption)
			selectedIface = network.Interface
		}

	default:
		log.Printf("Unknown network type: %T", selectedNetwork)
	}

	allInterfaces, err := net.Interfaces()

	if err != nil {
		log.Printf("Failed to fetch system interfaces: %s", err)
		return err
	}

	interfaceExists := false
	for _, iface := range allInterfaces {
		if iface.Name == selectedIface {
			interfaceExists = true
			break
		}
	}

	if !interfaceExists {
		return fmt.Errorf("interface %s does not exist", selectedIface)
	}

	ns := t.sm.Get().Network
	ns.PendingNetwork = selectedNetwork
	t.sm.SetNetwork(ns)
	return t.sm.Save()
}

func (t NetworkManagerLinux) TryConnect(nixPatch dogeboxd.NixPatch) error {
	state := t.sm.Get().Network

	if state.PendingNetwork == nil {
		return errors.New("no pending network to connect to")
	}

	connector := network_connector.NewNetworkConnector(state.PendingNetwork)

	err := connector.Connect(state.PendingNetwork)
	if err != nil {
		return err
	}

	// Create an instance of our network persistor, we do this here
	// because depending on the type of network we want (ethernet/wifi)
	// may result in a different persistor-type being used.
	persistor, err := network_persistor.NewNetworkPersistor(t.nix, state.PendingNetwork)
	if err != nil {
		return err
	}

	persistor.Persist(nixPatch, state.PendingNetwork)

	// Swap out pending for current.
	state.CurrentNetwork = state.PendingNetwork
	state.PendingNetwork = nil

	t.sm.SetNetwork(state)

	err = t.sm.Save()
	if err != nil {
		return err
	}

	log.Printf("Successfully saved network configuration to disk")
	return nil
}
