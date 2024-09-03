package system

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system/nix"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager, nixManager nix.NixManager, sourceManager dogeboxd.SourceManager, pupManager dogeboxd.PupManager) SystemUpdater {
	return SystemUpdater{
		config:     config,
		jobs:       make(chan dogeboxd.Job),
		done:       make(chan dogeboxd.Job),
		network:    networkManager,
		nix:        nixManager,
		sources:    sourceManager,
		pupManager: pupManager,
	}
}

type SystemUpdater struct {
	config     dogeboxd.ServerConfig
	jobs       chan dogeboxd.Job
	done       chan dogeboxd.Job
	network    dogeboxd.NetworkManager
	nix        nix.NixManager
	sources    dogeboxd.SourceManager
	pupManager dogeboxd.PupManager
}

func (t SystemUpdater) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
			dance:
				select {
				case <-stop:
					break mainloop
				case j, ok := <-t.jobs:
					if !ok {
						break dance
					}
					switch a := j.A.(type) {
					case dogeboxd.InstallPup:
						fmt.Printf("JON %+v", a)
						err := t.installPup(a, *j.State)
						if err != nil {
							fmt.Println("Failed to install pup", err)
							j.Err = "Failed to install pup"
						}
						t.done <- j
					case dogeboxd.UninstallPup:
						err := t.uninstallPup(*j.State)
						if err != nil {
							fmt.Println("Failed to uninstall pup", err)
							j.Err = "Failed to uninstall pup"
						}
						t.done <- j
					case dogeboxd.PurgePup:
						err := t.purgePup(*j.State)
						if err != nil {
							fmt.Println("Failed to purge pup", err)
							j.Err = "Failed to purge pup"
						}
						t.done <- j
					case dogeboxd.EnablePup:
						err := t.enablePup(*j.State)
						if err != nil {
							fmt.Println("Failed to enable pup", err)
							j.Err = "Failed to enable pup"
						}
						t.done <- j
					case dogeboxd.DisablePup:
						err := t.disablePup(*j.State)
						if err != nil {
							fmt.Println("Failed to disable pup", err)
							j.Err = "Failed to disable pup"
						}
						t.done <- j
					case dogeboxd.UpdatePendingSystemNetwork:
						err := t.network.SetPendingNetwork(*&a.Network)
						if err != nil {
							fmt.Println("Failed to set system network:", err)
							j.Err = "Failed to set system network"
						}
						t.done <- j
					default:
						fmt.Printf("Unknown action type: %v\n", a)
					}
				}
			}
		}()
		started <- true
		<-stop
		// do shutdown things
		stopped <- true
	}()
	return nil
}

func (t SystemUpdater) AddJob(j dogeboxd.Job) {
	fmt.Printf("add job %+v\n", j)
	t.jobs <- j
}

func (t SystemUpdater) GetUpdateChannel() chan dogeboxd.Job {
	return t.done
}

/* InstallPup takes a PupManifest and ensures a nix config
 * is written and any packages installed so that the Pup can
 * be started.
 */
func (t SystemUpdater) installPup(pupSelection dogeboxd.InstallPup, s dogeboxd.PupState) error {
	// TODO: Install deps!

	log.Printf("Installing pup from %s: %s @ %s", pupSelection.SourceName, pupSelection.PupName, pupSelection.PupVersion)
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)

	log.Printf("Downloading pup to %s", pupPath)
	err := t.sources.DownloadPup(pupPath, pupSelection.SourceName, pupSelection.PupName, pupSelection.PupVersion)
	if err != nil {
		log.Printf("Failed to download pup: %w", err)
		return err
	}

	storagePath := filepath.Join(t.config.DataDir, "pups/storage", s.ID)

	log.Printf("Creating pup storage directory: %s", storagePath)
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		log.Printf("Failed to create pup storage directory: %w", err)
		return err
	}

	// Now that we're mostly installed, enable it.
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Printf("Failed to update pup enabled state: %w", err)
		return err
	}

	log.Printf("Writing nix pup container config")
	if err := t.nix.WritePupFile(newState); err != nil {
		log.Printf("Failed to write nix pup file: %w", err)
		return err
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Printf("Failed to update pup installation state: %w", err)
		return err
	}

	log.Printf("Rebuilding nix")
	return t.nix.Rebuild()
}

func (t SystemUpdater) uninstallPup(s dogeboxd.PupState) error {
	// TODO: uninstall deps if they're not needed by another pup.

	log.Printf("Uninstalling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLING)); err != nil {
		log.Printf("Failed to update pup installation state: %w", err)
		return err
	}

	// Remove nix container configuration
	if err := t.nix.RemovePupFile(s.ID); err != nil {
		log.Printf("Failed to remove nix include file: %w", err)
		return err
	}

	// Update our core nix include file
	if err := t.nix.UpdateIncludeFile(t.pupManager); err != nil {
		log.Printf("Failed to update nix include file: %w", err)
		return err
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLED)); err != nil {
		log.Printf("Failed to update pup installation state: %w", err)
		return err
	}

	return t.nix.Rebuild()
}

func (t SystemUpdater) purgePup(s dogeboxd.PupState) error {
	log.Printf("Purging pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if s.Installation != dogeboxd.STATE_UNINSTALLED {
		log.Printf("Cannot purge pup %s in state %s", s.ID, s.Installation)
		return fmt.Errorf("Cannot purge pup %s in state %s", s.ID, s.Installation)
	}

	if err := t.pupManager.PurgePup(s.ID); err != nil {
		log.Printf("Failed to purge pup %s: %w", s.ID, err)
		return err
	}

	return nil
}

func (t SystemUpdater) enablePup(s dogeboxd.PupState) error {
	log.Printf("Enabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		return err
	}

	return t.nix.WritePupFile(newState)
}

func (t SystemUpdater) disablePup(s dogeboxd.PupState) error {
	log.Printf("Disabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(false))
	if err != nil {
		return err
	}

	return t.nix.WritePupFile(newState)
}
