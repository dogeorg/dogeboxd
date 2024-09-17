{ config, pkgs, lib, ... }:

{
  boot.enableContainers = true;

  systemd.tmpfiles.rules = [
    "d /var/log/containers 0750 root dogebox -"
  ];

  networking.nat = {
    enable = true;
    internalInterfaces = [ "ve-pup-+" ];
    enableIPv6 = false;
  };

  networking.firewall = {
    enable = true;
    extraCommands = ''
      # Block commands must live at the start of this template, as they're inserted (`-I`) at
      # the FRONT of the chain. As rules are evaluated 0-->..N for an ACCEPT, we insert
      # everything allowed at the front of the chain (before blocks) so it all works.

      # Block all other traffic within {{ .DOGEBOX_CONTAINER_CIDR }}
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT

      # Block everything else.
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} ! -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT

      # Allow traffic to {{ .DOGEBOX_HOST_IP }} (host)
      iptables -I FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_HOST_IP }} -j ACCEPT
      iptables -I FORWARD -s {{ .DOGEBOX_HOST_IP }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j ACCEPT

      {{- range .PUPS_TCP_CONNECTIONS }}
        {{- $PUP := . }}
        {{- range $PUP.OTHER_PUPS }}
          {{- $OTHER_PUP := . }}
          {{- range .PORTS }}
      # Connection FROM {{$PUP.ID}} ({{$PUP.NAME}}) to {{$OTHER_PUP.ID}} ({{$OTHER_PUP.NAME}})
      iptables -I FORWARD -p tcp -s {{$PUP.IP}} -d {{$OTHER_PUP.IP}} --dport {{.PORT}} -j ACCEPT

      # Connection BACK TO {{$PUP.ID}} ({{$PUP.NAME}}) from {{$OTHER_PUP.ID}} ({{$OTHER_PUP.NAME}})
      iptables -I FORWARD -p tcp -s {{$OTHER_PUP.IP}} -d {{$PUP.IP}} --sport {{.PORT}} -j ACCEPT
          {{- end}}
        {{- end}}
      {{- end}}

      {{ range .PUPS_REQUIRING_INTERNET }}
      # Explicitly block everything from {{.PUP_ID}} to all other pups.
      iptables -I FORWARD -s {{ .PUP_IP }} -d {{ $.DOGEBOX_CONTAINER_CIDR }} -j REJECT
      # But allow {{.PUP_ID}} to talk to everything else (ie. the internet)
      iptables -I FORWARD -s {{ .PUP_IP }} -j ACCEPT
      {{end}}
    '';
  };
}
