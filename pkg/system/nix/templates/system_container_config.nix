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
      iptables -A FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_HOST_IP }} -j ACCEPT
      iptables -A FORWARD -s {{ .DOGEBOX_HOST_IP }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j ACCEPT

      {{ range .PUPS_TCP_CONNECTIONS }}
        {{ $PUP := . }}
        {{ range $PUP.OTHER_PUPS }}
          {{ $OTHER_PUP := . }}
          {{ range $OTHER_PUP.PORTS }}
            # Connection FROM {{$PUP.ID}} ({{$PUP.NAME}}) to {{$OTHER_PUP.ID}} ({{$OTHER_PUP.NAME}})
            iptables -A FORWARD -p tcp -s {{$PUP.IP}} -d {{$OTHER_PUP.IP} --dport {{.PORT}} -j ACCEPT

            # Connection BACK TO {{$PUP.ID}} ({{$PUP.NAME}}) from {{$OTHER_PUP.ID}} ({{$OTHER_PUP.NAME}})
            iptables -A FORWARD -p tcp -s {{$OTHER_PUP.IP}} -d {{$PUP.IP}} --sport {{.PORT}} -j ACCEPT
          {{end}}
        {{end}}
      {{end}}

      {{ range .PUPS_REQUIRING_INTERNET }}
      # Explicitly block everything from {{.PUP_ID}} to all other pups.
      iptables -A FORWARD -s {{ .PUP_IP }} -d {{ $.DOGEBOX_CONTAINER_CIDR }} -j REJECT
      # But allow {{.PUP_ID}} to talk to everything else (ie. the internet)
      iptables -A FORWARD -s {{ .PUP_IP }} -j ACCEPT
      {{end}}

      # Block all other traffic within {{ .DOGEBOX_CONTAINER_CIDR }}
      iptables -A FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT

      # Block everything else.
      iptables -A FORWARD -s {{ .DOGEBOX_CONTAINER_CIDR }} ! -d {{ .DOGEBOX_CONTAINER_CIDR }} -j REJECT
    '';
  };
}
