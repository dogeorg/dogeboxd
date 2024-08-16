package network_persistor

import (
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NetworkPersistor = &NetworkPersistorNix{}

type NetworkPersistorNix struct{}

func (t NetworkPersistorNix) Persist(network dogeboxd.SelectedNetwork) {

}
