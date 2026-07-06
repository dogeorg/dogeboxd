{ config, lib, pkgs, ... }:

{
  imports =
    # Core system modules
    [
      ./system.nix
      ./firewall.nix
      ./network.nix
      ./system_container_config.nix
    ]
    # Optional custom configuration (only if it has been created)
    ++ lib.optionals (builtins.pathExists {{ .DATA_DIR }}/custom.nix) [
      {{ .DATA_DIR }}/custom.nix
    ]
    # Optional storage overlay (only if present in the nix dir)
    ++ lib.optionals (builtins.pathExists "{{ .NIX_DIR }}/storage-overlay.nix") [
      {{ .NIX_DIR }}/storage-overlay.nix
    ]
    # Optional pup containers (only if their nix files exist)
    {{range .PUP_IDS}}++ lib.optionals (builtins.pathExists ./pup_{{.}}.nix) [ ./pup_{{.}}.nix ]
    {{end}}
    ;
}
