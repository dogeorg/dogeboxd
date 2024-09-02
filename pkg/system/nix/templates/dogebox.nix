{ pkgs ? import <nixpkgs> {} }:

let
  system = import ./system.nix { inherit pkgs; };
  firewall = import ./firewall.nix { inherit pkgs; };
  recovery_ap = import ./recovery_ap.nix { inherit pkgs; };
  containers = import ./system_container_config.nix { inherit pkgs; };

  {{range .PUP_IDS}}pup_{{.}} = import ./pup_{{.}}.nix { inherit pkgs; };
  {{end}}
in
{
  inherit firewall containers system recovery_ap;
  {{range .PUP_IDS}}inherit pup_{{.}};
  {{end}}
}
