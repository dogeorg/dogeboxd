package network_connector

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NetworkConnector = &NetworkConnectorWPASupplicant{}

type NetworkConnectorWPASupplicant struct{}

func (t NetworkConnectorWPASupplicant) Connect(network dogeboxd.SelectedNetwork) {
	switch network.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		{
			log.Fatalf("Instantiated NetworkConnectorWPASupplicant for an ethernet network, aborting")
			return
		}
	}

	n := network.(dogeboxd.SelectedNetworkWifi)

	// Prepare wpa_supplicant command with network information
	cmd := exec.Command("wpa_supplicant",
		"-i", n.Interface,
		"-c", "/dev/null",
		"-C", "/var/run/wpa_supplicant",
		"-B",
		"-o", "/var/log/wpa_supplicant.log",
		"-D", "nl80211,wext",
	)

	// Set environment variables for network configuration
	cmd.Env = append(cmd.Env,
		"WPA_CTRL_INTERFACE=/var/run/wpa_supplicant",
		"WPA_CTRL_INTERFACE_GROUP=0",
	)

	// Start wpa_supplicant
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Failed to start wpa_supplicant: %v", err)
		return
	}

	log.Printf("Started wpa_supplicant for interface: %s", n.Interface)

	// Use wpa_cli to add and connect to the network
	addNetworkCmd := exec.Command("wpa_cli", "-i", n.Interface, "add_network")
	networkID, err := addNetworkCmd.Output()
	if err != nil {
		log.Fatalf("Failed to add network: %v", err)
		return
	}

	id := string(networkID)

	setSSIDCmd := exec.Command("wpa_cli", "-i", n.Interface, "set_network", id, "ssid", fmt.Sprintf("\"%s\"", n.Ssid))
	err = setSSIDCmd.Run()
	if err != nil {
		log.Fatalf("Failed to set SSID: %v", err)
		return
	}

	setPSKCmd := exec.Command("wpa_cli", "-i", n.Interface, "set_network", id, "psk", fmt.Sprintf("\"%s\"", n.Password))
	err = setPSKCmd.Run()
	if err != nil {
		log.Fatalf("Failed to set PSK: %v", err)
		return
	}

	enableNetworkCmd := exec.Command("wpa_cli", "-i", n.Interface, "enable_network", id)
	err = enableNetworkCmd.Run()
	if err != nil {
		log.Fatalf("Failed to enable network: %v", err)
		return
	}

	log.Printf("Attempting to connect to WiFi network: %s", n.Ssid)

	time.Sleep(20 * time.Second)

	// Check connection status
	statusCmd := exec.Command("wpa_cli", "-i", n.Interface, "status")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		log.Fatalf("Failed to get connection status: %v", err)
		return
	}

	status := string(statusOutput)

	if strings.Contains(status, "wpa_state=COMPLETED") {
		log.Printf("Successfully connected to WiFi network: %s", n.Ssid)
	} else {
		log.Printf("Failed to connect to WiFi network: %s. Current status: %s", n.Ssid, status)
	}
}
