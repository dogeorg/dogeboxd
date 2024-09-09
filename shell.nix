{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = [
    pkgs.systemd.dev
    pkgs.go_1_22
  ];

  shellHook = ''
    export PS1="\[\e[32m\]dogeboxd-env:\w \$\[\e[0m\] "
    export LD_LIBRARY_PATH=$(nix-build '<nixpkgs>' -A systemd)/lib:$LD_LIBRARY_PATH
  '';
}
