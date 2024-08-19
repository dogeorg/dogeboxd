package network_connector

import (
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewNetworkConnector(network dogeboxd.SelectedNetwork) dogeboxd.NetworkConnector {
	switch network.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		return NetworkConnectorEthernet{}
	case dogeboxd.SelectedNetworkWifi:
		return NetworkConnectorWPASupplicant{}
	default:
		log.Fatalf("No network connector specified for network: %+v", network)
		return nil
	}
}
