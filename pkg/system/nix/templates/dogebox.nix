{ pkgs ? import <nixpkgs> {} }:

let
  firewall = import ./firewall.nix { inherit pkgs; };
  containers = import ./system_container_config.nix { inherit pkgs; };
  system = import ./system.nix { inherit pkgs; };
in
{
  inherit firewall containers system;
}
