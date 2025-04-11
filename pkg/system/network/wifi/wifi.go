package network_wifi

type ScannedWifiNetwork struct {
	SSID       string
	BSSID      string
	Encryption string
	Quality    float32
	Signal     string
}

type WifiScanner interface {
	Scan(networkInterface string) ([]ScannedWifiNetwork, error)
}

func NewWifiScanner() WifiScanner {
	// TODO: Do some system discovery and figure out how to init this properly.
	return IWListScanner{}
}
