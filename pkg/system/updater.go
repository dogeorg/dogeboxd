package system

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager, nixManager dogeboxd.NixManager, sourceManager dogeboxd.SourceManager, pupManager dogeboxd.PupManager) SystemUpdater {
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
	nix        dogeboxd.NixManager
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
						err := t.network.SetPendingNetwork(a.Network)
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
	nixPatch := t.nix.NewPatch()

	// TODO: Install deps!
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_INSTALLING)); err != nil {
		log.Printf("Failed to update pup installation state: %w", err)
		return err
	}

	log.Printf("Installing pup from %s: %s @ %s", pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)

	log.Printf("Downloading pup to %s", pupPath)
	err := t.sources.DownloadPup(pupPath, pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	if err != nil {
		log.Printf("Failed to download pup: %w", err)
		return err
	}

	// Ensure the nix file configured in the manifest matches the hash specified.
	// Read pupPath s.Manifest.Container.Build.NixFile and hash it with sha256
	nixFile, err := os.ReadFile(filepath.Join(pupPath, s.Manifest.Container.Build.NixFile))
	if err != nil {
		log.Printf("Failed to read specified nix file: %w", err)
		return err
	}
	nixFileSha256 := sha256.Sum256(nixFile)

	// Compare the sha256 hash of the nix file to the hash specified in the manifest
	if fmt.Sprintf("%x", nixFileSha256) != s.Manifest.Container.Build.NixFileSha256 {
		log.Printf("Nix file hash mismatch")

		// Transition pup into a broken state. We probably need a "why" somewhere to convey to the user.
		_, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_BROKEN))
		if err != nil {
			log.Printf("Failed to transition pup into broken installation state: %w", err)
			return err
		}

		return fmt.Errorf("Nix file hash mismatch")
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "create-storage", "--data-dir", t.config.DataDir, "--pupId", s.ID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to create pup storage: %v. Command output: %s", err, output)
		return fmt.Errorf("failed to create pup storage: %w", err)
	}

	// Now that we're mostly installed, enable it.
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Printf("Failed to update pup enabled state: %w", err)
		return err
	}

	t.nix.WritePupFile(nixPatch, newState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	// Do a nix rebuild before we mark the pup as installed, this way
	// the frontend will get a much longer "Installing.." state, as opposed
	// to a much longer "Starting.." state, which might confuse the user.
	if err := nixPatch.Apply(); err != nil {
		log.Printf("Failed to apply nix patch: %w", err)
		return err
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Printf("Failed to update pup installation state: %w", err)
		return err
	}

	return nil
}

func (t SystemUpdater) uninstallPup(s dogeboxd.PupState) error {
	// TODO: uninstall deps if they're not needed by another pup.

	nixPatch := t.nix.NewPatch()

	log.Printf("Uninstalling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLING)); err != nil {
		log.Printf("Failed to update pup uninstalling state: %w", err)
		return err
	}

	t.nix.RemovePupFile(nixPatch, s.ID)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Printf("Failed to apply nix patch: %w", err)
		return err
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLED)); err != nil {
		log.Printf("Failed to update pup installation state: %w", err)
		return err
	}

	return nil
}

func (t SystemUpdater) purgePup(s dogeboxd.PupState) error {
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_PURGING)); err != nil {
		log.Printf("Failed to update pup purging state: %w", err)
		return err
	}

	pupDir := filepath.Join(t.config.DataDir, "pups")

	log.Printf("Purging pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if s.Installation != dogeboxd.STATE_UNINSTALLED {
		log.Printf("Cannot purge pup %s in state %s", s.ID, s.Installation)
		return fmt.Errorf("Cannot purge pup %s in state %s", s.ID, s.Installation)
	}

	// Delete pup state from disk
	if err := os.Remove(filepath.Join(pupDir, fmt.Sprintf("pup_%s.gob", s.ID))); err != nil {
		fmt.Println("Failed to remove pup state", err)
		return err
	}

	// Delete downloaded pup source
	if err := os.RemoveAll(filepath.Join(pupDir, s.ID)); err != nil {
		fmt.Println("Failed to remove pup source", err)
		return err
	}

	// Delete pup storage directory
	cmd := exec.Command("sudo", "_dbxroot", "pup", "delete-storage", "--pupId", s.ID, "--data-dir", t.config.DataDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("Failed to remove pup storage:", err)
		return err
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

	nixPatch := t.nix.NewPatch()
	t.nix.WritePupFile(nixPatch, newState)

	if err := nixPatch.Apply(); err != nil {
		log.Printf("Failed to apply nix patch: %w", err)
		return err
	}

	return nil
}

func (t SystemUpdater) disablePup(s dogeboxd.PupState) error {
	log.Printf("Disabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(false))
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error executing _dbxroot pup stop:", err)
		return err
	}

	nixPatch := t.nix.NewPatch()
	t.nix.WritePupFile(nixPatch, newState)

	if err := nixPatch.Apply(); err != nil {
		log.Printf("Failed to apply nix patch: %w", err)
		return err
	}

	return nil
}
