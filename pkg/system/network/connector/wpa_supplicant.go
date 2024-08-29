package network_connector

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NetworkConnector = &NetworkConnectorWPASupplicant{}

type NetworkConnectorWPASupplicant struct{}

func (t NetworkConnectorWPASupplicant) Connect(network dogeboxd.SelectedNetwork) error {
	switch network.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		{
			return errors.New("instantiated NetworkConnectorWPASupplicant for an ethernet network, aborting")
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

	// // Set environment variables for network configuration
	cmd.Env = append(cmd.Env,
		"WPA_CTRL_INTERFACE=/var/run/wpa_supplicant",
		"WPA_CTRL_INTERFACE_GROUP=0",
	)

	// // Start wpa_supplicant
	err := cmd.Start()
	if err != nil {
		log.Printf("failed to start wpa_supplicant for interface %s, %+v", n.Interface, err)
		return err
	}

	log.Printf("Started wpa_supplicant for interface: %s", n.Interface)

	// Use wpa_cli to add and connect to the network
	addNetworkCmd := exec.Command("wpa_cli", "-i", n.Interface, "add_network")
	networkID, err := addNetworkCmd.CombinedOutput()
	if err != nil {
		log.Printf("failed to add network: %+v", err)
		log.Print(string(networkID))
		return err
	}

	id := string(networkID)

	setSSIDCmd := exec.Command("wpa_cli", "-i", n.Interface, "set_network", id, "ssid", fmt.Sprintf("\"%s\"", n.Ssid))
	err = setSSIDCmd.Run()
	if err != nil {
		log.Printf("failed to set SSID: %v", err)
		return err
	}

	setPSKCmd := exec.Command("wpa_cli", "-i", n.Interface, "set_network", id, "psk", fmt.Sprintf("\"%s\"", n.Password))
	err = setPSKCmd.Run()
	if err != nil {
		log.Printf("failed to set PSK: %v", err)
		return err
	}

	enableNetworkCmd := exec.Command("wpa_cli", "-i", n.Interface, "enable_network", id)
	err = enableNetworkCmd.Run()
	if err != nil {
		log.Printf("failed to enable network: %v", err)
		return err
	}

	log.Printf("Attempting to connect to WiFi network: %s", n.Ssid)

	time.Sleep(20 * time.Second)

	// Check connection status
	statusCmd := exec.Command("wpa_cli", "-i", n.Interface, "status")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		log.Printf("Failed to get connection status: %v", err)
		return err
	}

	status := string(statusOutput)

	if strings.Contains(status, "wpa_state=COMPLETED") {
		log.Printf("Successfully connected to WiFi network: %s", n.Ssid)
	} else {
		log.Printf("Failed to connect to WiFi network: %s. Current status: %s", n.Ssid, status)
	}

	return nil
}
