{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = [
    pkgs.systemd.dev
    pkgs.go_1_22
  ];

  shellHook = ''
    export PS1="\[\e[32m\]dogeboxd-env:\w \$\[\e[0m\] "
  '';
}
