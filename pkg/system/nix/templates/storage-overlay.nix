{ ... }:

{
  fileSystems."/opt" = {
    device = "{{ .STORAGE_DEVICE }}";
    fsType = "ext4";
    mountPoint = "/opt";
  };
}
