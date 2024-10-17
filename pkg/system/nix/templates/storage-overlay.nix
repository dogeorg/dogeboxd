{ pkgs, ... }:

{
  # Ideally we'd use nix .fileSystems.<name> here, but it doesn't seem to work?

  systemd.services.mount-data-overlay = {
    description = "Mounts the selected storage device as an overlay at {{.DATA_DIR}}";
    wantedBy = [ "local-fs.target" ];
    script = ''
      ${pkgs.mount}/bin/mount {{ .STORAGE_DEVICE }} {{ .DATA_DIR }}
      ${pkgs.coreutils}/bin/chown {{.DBX_UID}}:{{.DBX_UID}} {{.DATA_DIR}}
      ${pkgs.coreutils}/bin/chmod u+rwX,g+rwX,o-rwx {{ .DATA_DIR }}
    '';
  };
}
