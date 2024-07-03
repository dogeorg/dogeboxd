![Dogebox Logo](/docs/dogebox-logo.png)

# dogeboxd

## Dogeboxd is the Dogebox OS system manager service.

### A Daemon 

Dogeboxd runs as a daemon under systemd and acts as the parent process for all 'Dogebox Apps' which it runs 
within Linux namespaces to provide appropriate process isolation and access to the Dogebox Runtime Environment. 
In a way Dogeboxd is equivilent to Dockerd, it is responsible for containerisation and orchestration
of processes in secure contexts, however it is more than that:

### A Package Manager

Dogeboxd operates as a package-manager for 'Dogebox Apps': NixOS packaged services and web-apps with a Dogebox 
Manifest that describes the Dogecoin services and OS requirements they need. These are discoverable via Dogebox
package manifests, JSON files served over the web. What this means is anyone can distribute software for Dogebox
and provide a package manifest that describes their requirements, that users can then click to install and configure.

### An Environment

Dogeboxd provides an app environment (DRE - Dogebox Runtime Environment) which is a curated set of APIs exposed via 
local NGINX proxy, so that apps can be developed against a known set of extensible, stable APIs.  For example, you can 
specify your app needs 'core' and 'gigawallet' in your app manifest, and within your process environment core.localhost 
and gigawallet.localhost will be available respectively. Versioned dependencies will be able to be specified in your app
manifest, but ultimate control of what is running will be in the hands of the Dogebox owner.

Major features of the DRE which are under construction:

 - NGINX proxy for all 'services' available after the user 'approves access' at app install time.
 - A doge-walker API that issues a realtime stream of blocks you can subscribe to, accounting for tip-changes and rollbacks.
 - A transaction & block index API which allows quick search of the blockchain, useful for building block explorers.
 - Extensible API surface via plugins that will introduce new APIs built by the community to the runtime. 
   
The aim of the Dogebox Daemon is to provide a secure environment for Shibes to try out various kinds of software from 
the Dogecoin Ecosystem, in a way that puts them in control, without the need to be linux sysadmin. We want to see 
every member of the Dogecoin community able to contribute to the network, but also to provide a platform where devs 
can share their work with the whole community also. 



