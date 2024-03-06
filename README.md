![Dogebox Logo](/docs/dogebox-logo.png)

# dogeboxd

Dogeboxd is the Dogebox OS system manager service.

### A Daemon 

Dogeboxd operates as a daemon under systemd and acts as the parent process for 'Dogebox Apps' which are launched 
within a Linux namespace to provide appropriate process isolation and access to the Dogebox runtime environment. 
You can think of it as the equivilent of Dockerd in that it is responsible for containerisation and orchestration
of processes.

### A Package Manager

Dogeboxd operates as a meta-package-manager for 'Dogebox Apps', which are discoverable via dogebox-package-manifest
JSON files served over the web. What this means is anyone can distribute software for Dogebox and provide a package
manifest that describes it's requirements, that users can then click to install and configure.

### An Environment

Dogeboxd curates an app environment (DRE - Dogebox Runtime Environment) which is a curated set of APIs exposed via 
a local NGINX proxy, so that apps can be developed against a known set of extensible APIs.  Developers can specify 
their app needs 'core' and will then be able to connect to core.localhost/<core api>, or gigawallet.localhost/<some api>



