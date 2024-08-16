package network_connector

import dogeboxd "github.com/dogeorg/dogeboxd/pkg"

func NewNetworkConnector(network dogeboxd.SelectedNetwork) dogeboxd.NetworkConnector {
	// TODO: Do some system discovery and figure out how to init this properly.
	return NetworkConnectorWPASupplicant{}
}
