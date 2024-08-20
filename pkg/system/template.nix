{ config, libs, pkgs, ... }:

{
  containers.pups-{{.PUP_SLUG}} = {

    # If our pup is enabled, we set it to autostart on boot.
    autoStart = {{.PUP_ENABLED}};

    # Set up private networking. This will ensure the pup gets an internal IP
    # in the range of 10.0.0.0/8, be able to to dogeboxd at 10.0.0.1, but not
    # be able to talk to any other pups without proxying through dogeboxd.
    privateNetwork = true;
    hostAddress = "10.0.0.1";
    localAddress = {{.INTERNAL_IP}}

    # Mount somewhere that can be used as storage for the pup.
    # The rest of the filesystem is marked as readonly (and ephemeral)
    bindMounts = {
      "Persistent Storage" = {
        mountPoint = "/storage";
        hostPath = "/pups/{{.PUP_SLUG}}";
        isReadOnly = false;
      };
    };

    ephemeral = true;

    config = { config, pkgs, lib, ... }: {
      system.stateVersion = "24.05";
      system.copySystemConfiguration = true;

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
          allowedTCPPorts = [ {{ .PUP_PORTS }} ];
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

      # TODO. Just echo test on port 80 for the moment :)
      environment.systemPackages = [ pkgs.socat ];

      systemd.services.simple-http = {
        description = "Simple HTTP server returning 'test'";
        after = [ "network.target" ];
        wantedBy = [ "multi-user.target" ];

        serviceConfig = {
          ExecStart = "${pkgs.socat}/bin/socat TCP-LISTEN:80,fork EXEC:'echo -e \"HTTP/1.1 200 OK\\r\\nContent-Length: 4\\r\\n\\r\\ntest\"'";
          Restart = "always";
        };
      };
    };
  };
}  
