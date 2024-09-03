{ config, lib, pkgs, ... }:

{
  imports = [
    ./system.nix
    ./firewall.nix
    # ./recovery_ap.nix
    ./system_container_config.nix
    {{range .PUP_IDS}}./pup_{{.}}.nix
    {{end}}
  ];
}
