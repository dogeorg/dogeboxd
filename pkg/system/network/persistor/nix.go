package network_persistor

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NetworkPersistor = &NetworkPersistorNix{}

type NetworkPersistorNix struct{}

//go:embed nix_template_wifi.nix
var nixWifiTemplate []byte

type nixWifiTemplateValues struct {
	INTERFACE     string
	WIFI_SSID     string
	WIFI_PASSWORD string
}

//go:embed nix_template_ethernet.nix
var nixEthernetTemplate []byte

type nixEthernetTemplateValues struct {
	INTERFACE string
}

// TODO?
const NIX_SYSTEM_PATH = "/etc/nixos/dogebox"

func (t NetworkPersistorNix) Persist(network dogeboxd.SelectedNetwork) error {
	switch network := network.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		{
			return persistEthernet(network)
		}
	case dogeboxd.SelectedNetworkWifi:
		{
			return persistWifi(network)
		}
	}
	return nil
}

func persistEthernet(network dogeboxd.SelectedNetworkEthernet) error {
	v := nixEthernetTemplateValues{
		INTERFACE: network.Interface,
	}

	t, err := template.New("nix_ethernet").Parse(string(nixEthernetTemplate))
	if err != nil {
		return err
	}

	return writeConfig(t, v)
}

func persistWifi(network dogeboxd.SelectedNetworkWifi) error {
	v := nixWifiTemplateValues{
		INTERFACE:     network.Interface,
		WIFI_SSID:     network.Ssid,
		WIFI_PASSWORD: network.Password,
	}

	t, err := template.New("nix_wifi").Parse(string(nixWifiTemplate))
	if err != nil {
		return err
	}

	return writeConfig(t, v)
}

func writeConfig(template *template.Template, values any) error {
	p := filepath.Join(NIX_SYSTEM_PATH, "network.nix")
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Failed to open NIX_SYSTEM_PATH for writing: %s", p)
		return err
	}
	defer f.Close()

	err = template.Execute(f, values)
	if err != nil {
		fmt.Printf("Failed to write wifi network template to NIX_SYSTEM_PATH: %+v", err)
		return err
	}

	log.Println("Wrote nix config to disk, rebuilding..")

	err = nixRebuild()
	if err != nil {
		return err
	}

	return nil
}

// TODO: dedupe this
func nixRebuild() error {
	md := exec.Command("nixos-rebuild", "switch")

	output, err := md.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing nix rebuild: %s\n", err)
		return err
	} else {
		fmt.Printf("nix output: %s\n", string(output))
	}
	return nil
}
