{ config, lib, pkgs, ... }:

{
  imports = [
    ./system.nix
    ./firewall.nix
    ./network.nix
    # ./recovery_ap.nix
    ./system_container_config.nix
  ]
  {{range .PUP_IDS}}++ lib.optionals (builtins.pathExists ./pup_{{.}}.nix) [ ./pup_{{.}}.nix ]
  {{end}}
  ;
}
