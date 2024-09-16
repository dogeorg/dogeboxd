{ config, pkgs, lib, ... }:

{
  boot.enableContainers = true;

  networking.nat = {
    enable = true;
    internalInterfaces = [ "ve-pup-+" ];
    enableIPv6 = false;
  };

  networking.firewall = {
    enable = true;
    extraCommands = ''
      # Allow traffic to {{ .DOGEBOX_HOST_IP }} (host)
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_HOST_IP }} -j ACCEPT
      iptables -I FORWARD -s {{ .DOGEBOX_HOST_IP }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j ACCEPT

      {{ range .PUPS_REQUIRING_INTERNET }}
      # Explicitly block everything from {{.PUP_ID}} to all other pups.
      iptables -I FORWARD -s {{ .PUP_IP }} -d {{ $.DOGEBOX_CONTAINER_CIDR }} -j REJECT
      # But allow {{.PUP_ID}} to talk to everything else (ie. the internet)
      iptables -I FORWARD -s {{ .PUP_IP }} -j ACCEPT
      {{end}}

      # Block all other traffic within {{ .DOGEBOX_CONTAINER_CIDR }}
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT

      # Block everything else.
      iptables -A FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} ! -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT
    '';
  };
}
