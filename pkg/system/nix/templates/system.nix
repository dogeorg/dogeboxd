{ config, pkgs, lib, ... }:

{
  networking.hostName = lib.mkForce "{{ .SYSTEM_HOSTNAME }}";
  networking.networkmanager.enable = false;

  services.openssh.settings = {
    AllowUsers = [ "shibe" ];
  };

  services.openssh.banner = ''
+===================================================+
|                                                   |
|      ____   ___   ____ _____ ____   _____  __     |
|     |  _ \ / _ \ / ___| ____| __ ) / _ \ \/ /     |
|     | | | | | | | |  _|  _| |  _ \| | | \  /      |
|     | |_| | |_| | |_| | |___| |_) | |_| /  \      |
|     |____/ \___/ \____|_____|____/ \___/_/\_\     |
|                                                   |
+===================================================+
'';

  services.openssh.enable = lib.mkForce {{ .SSH_ENABLED }};

  users.users.shibe = {
    isNormalUser = true;
    group = "wheel";
    openssh = {
      authorizedKeys = {
        keys = [
          {{ range .SSH_KEYS }}"{{.Key}} # {{.ID}}"{{ end }}
        ];
      };
    };
  };
}
