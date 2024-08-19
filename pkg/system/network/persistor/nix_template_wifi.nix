{ config, pkgs, ... }:

{
  networking = {
    wireless = {
      enable = true;
      interfaces = [ "{{.INTERFACE}}" ];
      networks = {
        "{{.WIFI_SSID}}" = {
          psk = "{{.WIFI_PASSWORD}}";
        };
      };
    };
  };
}
