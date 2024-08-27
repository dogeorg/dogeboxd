{ config, pkgs, lib, ... }:

{
  networking.firewall.enabled = true;

  networking.firewall.allowedTCPPorts = [
    {{ if .SSH_ENABLED }} 22 {{end}}
  ];
}
