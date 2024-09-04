{ config, pkgs, ... }:

{
  networking = {
    {{if .USE_ETHERNET}}
    interfaces = {
      {{.INTERFACE}} = {
        useDHCP = true;
      };
    };
    {{else if .USE_WIRELESS}}
    wireless = {
      enable = true;
      interfaces = [ "{{.INTERFACE}}" ];
      networks = {
        "{{.WIFI_SSID}}" = {
          psk = "{{.WIFI_PASSWORD}}";
        };
      };
    };
    {{end}}
  };
}
