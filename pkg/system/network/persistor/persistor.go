package network_persistor

import (
	"errors"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewNetworkPersistor(network dogeboxd.SelectedNetwork) (dogeboxd.NetworkPersistor, error) {
	// TODO: Do some system discovery and figure out how to init this properly.
	if isNix() {
		return NetworkPersistorNix{}, nil
	}

	return nil, errors.New("failed to intialise network persistor, no handler implemented")
}

func isNix() bool {
	return true
}
