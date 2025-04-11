package network_wifi

import (
	"bytes"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var _ WifiScanner = &IWListScanner{}

type IWListScanner struct{}

func (s IWListScanner) Scan(interfaceName string) ([]ScannedWifiNetwork, error) {
	cmd := exec.Command("sudo", "_dbxroot", "iwlist", interfaceName, "scan")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return parseIWListOutput(out.String()), nil
}

func parseIWListCell(cell string)  *ScannedWifiNetwork {
	ssidRegex := regexp.MustCompile(`ESSID:"(.*?)"`)
	addressRegex := regexp.MustCompile(`Address: ([0-9A-Fa-f:]+)`)
	encryptionRegex := regexp.MustCompile(`Encryption key:(on|off)`)
	wpaRegex := regexp.MustCompile(`IE: IEEE 802.11i/WPA2 Version`)
	wpa2Regex := regexp.MustCompile(`IE: WPA Version 1`)
	qualityRegex := regexp.MustCompile(`Quality=([0-9]+)/([0-9]+)`)
	signalRegex := regexp.MustCompile(`Signal level=(-?[0-9]+ dBm)`)

	ssid := ssidRegex.FindStringSubmatch(cell)
	address := addressRegex.FindStringSubmatch(cell)
	encryption := encryptionRegex.FindStringSubmatch(cell)
	wpa := wpaRegex.FindString(cell)
	wpa2 := wpa2Regex.FindString(cell)
	quality := qualityRegex.FindStringSubmatch(cell)
	signal := signalRegex.FindStringSubmatch(cell)

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

		qualityDividend, err := strconv.Atoi(quality[1])
		if err != nil {
			qualityDividend = 0
		}
		qualityDivisor, err := strconv.Atoi(quality[2])
		if err != nil {
			qualityDividend = 0
			qualityDivisor = 1
		}

		network := ScannedWifiNetwork{
			SSID:       ssid[1],
			BSSID:      address[1],
			Encryption: encryptionType,
			Quality: float32(qualityDividend)/float32(qualityDivisor),
			Signal: signal[1],
		}
		return &network
	}

	return nil
}

func parseIWListOutput(output string) []ScannedWifiNetwork {
	networkMap := make(map[string]ScannedWifiNetwork)
	cells := strings.Split(output, "Cell ")

	for _, cell := range cells {
		parsedNetwork := parseIWListCell(cell)

		if parsedNetwork == nil { continue }

		if network, ok := networkMap[parsedNetwork.SSID]; ok {
			if network.Quality < parsedNetwork.Quality {
				networkMap[parsedNetwork.SSID] = *parsedNetwork
			}
		} else {
			networkMap[parsedNetwork.SSID] = *parsedNetwork
		}
	}

	networks := maps.Values(networkMap)

	return slices.SortedStableFunc(networks, func(a, b ScannedWifiNetwork) int {
		return int(a.Quality - b.Quality)
	})
}
