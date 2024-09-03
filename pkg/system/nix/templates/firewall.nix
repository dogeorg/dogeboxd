{ config, pkgs, lib, ... }:

{
  networking.firewall.enable = true;

  networking.firewall.allowedTCPPorts = [
    {{ if .SSH_ENABLED }} 22 {{end}}
  ];
}
