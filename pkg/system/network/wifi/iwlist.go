package network_wifi

import (
	"bytes"
	"os/exec"
	"regexp"
	"strings"
)

var _ WifiScanner = &IWListScanner{}

type IWListScanner struct{}

func (s IWListScanner) Scan(interfaceName string) ([]ScannedWifiNetwork, error) {
	cmd := exec.Command("iwlist", interfaceName, "scan")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return parseIWListOutput(out.String()), nil
}

func parseIWListOutput(output string) []ScannedWifiNetwork {
	ssidRegex := regexp.MustCompile(`ESSID:"(.*?)"`)
	addressRegex := regexp.MustCompile(`Address: ([0-9A-Fa-f:]+)`)
	encryptionRegex := regexp.MustCompile(`Encryption key:(on|off)`)
	wpaRegex := regexp.MustCompile(`IE: IEEE 802.11i/WPA2 Version`)
	wpa2Regex := regexp.MustCompile(`IE: WPA Version 1`)

	var networks []ScannedWifiNetwork
	cells := strings.Split(output, "Cell ")

	for _, cell := range cells {
		ssid := ssidRegex.FindStringSubmatch(cell)
		address := addressRegex.FindStringSubmatch(cell)
		encryption := encryptionRegex.FindStringSubmatch(cell)
		wpa := wpaRegex.FindString(cell)
		wpa2 := wpa2Regex.FindString(cell)

		if len(ssid) > 1 && len(address) > 1 && len(encryption) > 1 {
			var encryptionType string

			if encryption[1] == "on" {
				if wpa != "" {
					encryptionType = "WPA2"
				} else if wpa2 != "" {
					encryptionType = "WPA"
				} else {
					encryptionType = "WEP"
				}
			}

			network := ScannedWifiNetwork{
				SSID:       ssid[1],
				BSSID:      address[1],
				Encryption: encryptionType,
			}
			networks = append(networks, network)
		}
	}

	return networks
}
