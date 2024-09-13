{ config, pkgs, lib, ... }:

{
  networking.firewall.enable = true;

  networking.firewall.allowedTCPPorts = [
    {{ if .SSH_ENABLED }}
    # TODO: Allow the user to customise this at some point.
    # Enable port 22 for OpenSSH
    22
    {{end}}
    {{ range .PUP_PORTS }}{{ if .PUBLIC }}
    # Open port {{.PORT}} (forwarding to {{.PORT}}) for pup {{.PUP_ID}}
    {{.PORT}}
    {{end}}{{end}}
  ];
}
