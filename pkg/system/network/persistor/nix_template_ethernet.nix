{ config, pkgs, ... }:

{
  networking = {
    interfaces = {
      {{.INTERFACE}} = {
        useDHCP = true;
      };
    };
  };
}
