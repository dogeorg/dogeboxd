{ config, pkgs, ... }:

let
  {{ range .PUPS }}
  {{ .PUP_NAME }} = import {{ .PUP_PATH }};
  {{ end }}
in
{
  nixpkgs.overlays = [
    {{ range .PUPS }}
    {{ .PUP_NAME }}
    {{ end }}
  ];
}
