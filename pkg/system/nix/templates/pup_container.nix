{ config, libs, pkgs, ... }:

let
  pupOverlay = self: super: {
    pup = import {{.NIX_FILE}} { inherit pkgs; };
  };
in
{
  # Maybe don't need this here at the top-level, only inside the container block?
  nixpkgs.overlays = [ pupOverlay ];

  containers.pup-{{.PUP_ID}} = {

    # If our pup is enabled, we set it to autostart on boot.
    autoStart = {{.PUP_ENABLED}};

    # Set up private networking. This will ensure the pup gets an internal IP
    # in the range of 10.0.0.0/8, be able to to dogeboxd at 10.0.0.1, but not
    # be able to talk to any other pups without proxying through dogeboxd.
    privateNetwork = true;
    hostAddress = "10.0.0.1";
    localAddress = "{{.INTERNAL_IP}}";

    # Mount somewhere that can be used as storage for the pup.
    # The rest of the filesystem is marked as readonly (and ephemeral)
    bindMounts = {
      "Persistent Storage" = {
        mountPoint = "/storage";
        hostPath = "{{ .STORAGE_PATH }}";
        isReadOnly = false;
      };

      "PUP" = {
        mountPoint = "/pup";
        hostPath = "{{ .PUP_PATH }}";
        isReadOnly = true;
      };
    };

    ephemeral = true;

    config = { config, pkgs, lib, ... }: {
      system.stateVersion = "24.05";
      system.copySystemConfiguration = true;

      nixpkgs.overlays = [ pupOverlay ];

      # Mark our root fs as readonly.
      fileSystems."/" = {
        device = "rootfs";
        options = [ "ro" ];
      };

      networking = {
        useHostResolvConf = lib.mkForce false;
        firewall = {
          enable = true;
          # If the pup has marked that is listens on ports
          # explicitly whitelist those in the container fw.
          allowedTCPPorts = [ {{ range .PUP_PORTS }}{{ . }} {{end}}];
        };
        hosts = {
          # Helper so you can always hit dogebox(d) in DNS.
          "10.0.0.1" = [ "dogeboxd" "dogeboxd.local" "dogebox" "dogebox.local" ];
        };
      };

      services.resolved.enable = true;

      # Create a group & user for running the pup executable as.
      users.groups.pup = {};
      users.users.pup = {
        isSystemUser = false;
        isNormalUser = true;
        group =  "pup";
      };

      environment.systemPackages = with pkgs; [
        {{ range .SERVICES }}pup.{{.NAME}} {{end}}
      ];

      {{range .SERVICES}}
      systemd.services.{{.NAME}} = {
        after = [ "network.target" ];
        wantedBy = [ "multi-user.target" ];

        serviceConfig = {
          ExecStart = "${pkgs.pup.{{.NAME}}}{{.EXEC}}";
          Restart = "always";
          User = "pup";
          Group = "pup";

          WorkingDirectory = "{{.CWD}}";

          Environment = [
            {{range .ENV}}"{{.KEY}}={{.VAL}}"{{end}}
          ];

          PrivateTmp = true;
          ProtectSystem = "full";
          ProtectHome = "yes";
          NoNewPrivileges = true;
        };
      };
      {{end}}
    };
  };
}  
