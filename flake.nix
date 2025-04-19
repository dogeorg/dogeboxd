{
  inputs = {
    nixpkgs.url     = "github:NixOS/nixpkgs/nixos-24.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        isLinux = builtins.match ".*-linux$" system != null;
      in {
        devShells.default = if isLinux then
          pkgs.mkShell {
            buildInputs = [
              pkgs.gnumake
              pkgs.systemd.dev
              pkgs.go_1_22
              pkgs.parted
              pkgs.util-linux
              pkgs.e2fsprogs
              pkgs.dosfstools
              pkgs.nixos-install-tools
              pkgs.nix
              pkgs.git
              pkgs.libxkbcommon
            ];
          }
        else
          pkgs.mkShell {
            shellHook = ''
              echo "🚫 Unsupported system: ${system}"
              echo "Dogeboxd development relies on systemd headers, which are only available on Linux. Please run in a VM."
              exit 1
            '';
          };

        dbxSessionName = "dogeboxd";
        dbxStartCommand = "make dev";
      }
    );
}
