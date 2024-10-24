{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = [
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

  shellHook = ''
    export PS1="\[\e[32m\]dogeboxd-env:\w \$\[\e[0m\] "
  '';
}
