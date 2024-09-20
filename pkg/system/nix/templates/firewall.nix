{ config, pkgs, lib, ... }:

{
  networking.firewall.enable = true;

  networking.firewall.allowedTCPPorts = [
    # Allow dogeboxd to bind to 80 for the pup-router.
    # TODO: Make this an explicit firewall rule only available to pup cide.
    80

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
