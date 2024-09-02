{ config, pkgs, ... }:

{
  services.create_ap = {
    enable = {{ .AP_ENABLED }};
    settings = {
      FREQ_BAND = "2.4";
      GATEWAY = "10.0.0.69";
      ISOLATE_CLIENTS = 1;
      WPA_VERSION = 2;
      # Bug in this service. This needs to be passed.
      INTERNET_IFACE = {{ .INTERFACE }};
      WIFI_IFACE = {{ .INTERFACE }};
      SSID = {{ .SSID }};
      PASSPHRASE = {{ .PASSWORD }};
    };
  };
}
