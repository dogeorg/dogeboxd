{ pkgs }:

let
  {{.PUP_NAME}}-service = pkgs.stdenv.mkDerivation {
    name = "{{.PUP_NAME}}_service";
    version = "1.0";

    buildInputs = [ pkgs.go ];

    unpackPhase = "true";

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      mkdir -p $out/bin
      go build -o $out/bin/server server.go
    '';

    installPhase = ''
      echo "Binary built at $out/bin/server"
    '';
  };

in
{
  inherit {{.PUP_NAME}}-service;
}
