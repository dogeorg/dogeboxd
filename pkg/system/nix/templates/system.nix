{ config, pkgs, lib, ... }:

{
  networking.hostname = "dogebox";
  networking.networkmanager.enable = false;

  services.openssh.settings = {
    AllowUsers = [ "dogebox" ];
  };

  services.openssh.banner = ""
+===================================================+
|                                                   |
|      ____   ___   ____ _____ ____   _____  __     |
|     |  _ \ / _ \ / ___| ____| __ ) / _ \ \/ /     |
|     | | | | | | | |  _|  _| |  _ \| | | \  /      |
|     | |_| | |_| | |_| | |___| |_) | |_| /  \      |
|     |____/ \___/ \____|_____|____/ \___/_/\_\     |
|                                                   |
+===================================================+
  "";

  services.openssh.enable = {{ .SSH_ENABLED }};

  users.groups.dogeboxd = {};
  users.users.dogeboxd = {
    isSystemUser = true;
    group =  "dogeboxd";

    openssh = {
      authorizedKeys = {
        keys = [
          {{ range .SSH_KEYS }}"{{.}}"{{ end }}
        ];
      };
    };
  };
}
