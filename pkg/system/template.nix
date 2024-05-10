# Include this from the main /etc/nixos/configuration.nix
# then include a 'services.SERVICE_NAME.enable = true;'

{ config, libs, pkgs, ... }:

let
  cfg = config.services.{{.SERVICE_NAME}};
in

#  pups = mapAttrs (
#
#  ) cfg.pups; # Still deciding if/how pup attributes are imported here

with lib;

{
  options = {
    services.{{.SERVICE_NAME}} = {
      enable = mkOption {
        default = false;
        type = with types; bool;
      };
  };

  config = mkIf cfg.enable {
    systemd.services.{{.SERVICE_NAME}} = {
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      description = "";
      serviceConfig = {
        Type = "simple";     # Whatever systemd service type makes sense
        User = "pup";
        ExecStart = "${pkgs.dogeboxd}{{.EXEC_COMMAND}}";
        # ExecStop = "${pkgs.dogeboxd}/path/to/file";
      };
    };
  };
}  
