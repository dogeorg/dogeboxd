package network_persistor

import (
	_ "embed"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NetworkPersistor = &NetworkPersistorNix{}

type NetworkPersistorNix struct {
	nix dogeboxd.NixManager
}

func (t NetworkPersistorNix) Persist(network dogeboxd.SelectedNetwork) error {
	values := dogeboxd.NixNetworkTemplateValues{}

	switch network := network.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		{
			values.INTERFACE = network.Interface
			values.USE_ETHERNET = true
			values.USE_WIRELESS = false
		}
	case dogeboxd.SelectedNetworkWifi:
		{
			values.INTERFACE = network.Interface
			values.USE_ETHERNET = false
			values.USE_WIRELESS = true
			values.WIFI_SSID = network.Ssid
			values.WIFI_PASSWORD = network.Password
		}
	}

	return t.nix.UpdateNetwork(values)
}
