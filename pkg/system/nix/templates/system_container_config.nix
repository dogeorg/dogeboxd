{ config, pkgs, lib, ... }:

{
  networking.nat = {
    enable = true;
    internalInterfaces = [ "ve-pups-+" ];
    enableIPv6 = false;
  };

  networking.firewall = {
    enable = true;
    extraCommands = ''
      # Allow traffic to {{ .DOGEBOX_HOST_IP }} (host)
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_HOST_IP }} -j ACCEPT
      iptables -I FORWARD -s {{ .DOGEBOX_HOST_IP }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j ACCEPT

      # Block all other traffic within {{ .DOGEBOX_CONTAINER_CIDR }}
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT

      # TODO: Only apply this when a pup hasn't explicitly requested
      #       internet access. Should be blocked by default
      iptables -A FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} ! -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT
    '';
  };
}
