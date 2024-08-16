package network

import (
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	network_wifi "github.com/dogeorg/dogeboxd/pkg/system/network/wifi"
)

func NewNetworkManager(sm dogeboxd.StateManager) dogeboxd.NetworkManager {
	// TODO: Do some system discovery and figure out how to init this properly.
	return NetworkManagerLinux{
		sm:          sm,
		WifiScanner: network_wifi.NewWifiScanner(),
	}
}
