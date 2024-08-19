package network_persistor

import (
	"errors"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewNetworkPersistor(network dogeboxd.SelectedNetwork) (dogeboxd.NetworkPersistor, error) {
	if isNix() {
		return NetworkPersistorNix{}, nil
	}

	return nil, errors.New("failed to intialise network persistor, no handler implemented")
}

func isNix() bool {
	return true
}
