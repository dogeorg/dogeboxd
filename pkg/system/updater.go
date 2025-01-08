package system

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/utils"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager, nixManager dogeboxd.NixManager, sourceManager dogeboxd.SourceManager, pupManager dogeboxd.PupManager, stateManager dogeboxd.StateManager, dkm dogeboxd.DKMManager) SystemUpdater {
	return SystemUpdater{
		config:     config,
		jobs:       make(chan dogeboxd.Job),
		done:       make(chan dogeboxd.Job),
		network:    networkManager,
		nix:        nixManager,
		sources:    sourceManager,
		pupManager: pupManager,
		sm:         stateManager,
		dkm:        dkm,
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
	sm         dogeboxd.StateManager
	dkm        dogeboxd.DKMManager
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
						err := t.installPup(a, j)
						if err != nil {
							j.Err = "Failed to install pup"
						}
						t.done <- j
					case dogeboxd.UninstallPup:
						err := t.uninstallPup(j)
						if err != nil {
							j.Err = "Failed to uninstall pup"
						}
						t.done <- j
					case dogeboxd.PurgePup:
						err := t.purgePup(j)
						if err != nil {
							j.Err = "Failed to purge pup"
						}
						t.done <- j
					case dogeboxd.EnablePup:
						err := t.enablePup(j)
						if err != nil {
							j.Err = "Failed to enable pup"
						}
						t.done <- j
					case dogeboxd.DisablePup:
						err := t.disablePup(j)
						if err != nil {
							j.Err = "Failed to disable pup"
						}
						t.done <- j
					case dogeboxd.UpdatePendingSystemNetwork:
						err := t.network.SetPendingNetwork(a.Network, j)
						if err != nil {
							j.Err = "Failed to set system network"
						}
						t.done <- j

					case dogeboxd.EnableSSH:
						err := t.EnableSSH(j.Logger.Step("enable SSH"))
						if err != nil {
							j.Err = "Failed to enable SSH"
						}
						t.done <- j
					case dogeboxd.DisableSSH:
						err := t.DisableSSH(j.Logger.Step("disable SSH"))
						if err != nil {
							j.Err = "Failed to disable SSH"
						}
						t.done <- j

					case dogeboxd.AddSSHKey:
						err := t.AddSSHKey(a.Key, j.Logger.Step("add SSH key"))
						if err != nil {
							j.Err = "Failed to add SSH key"
						}
						t.done <- j

					case dogeboxd.RemoveSSHKey:
						err := t.RemoveSSHKey(a.ID, j.Logger.Step("remove SSH key"))
						if err != nil {
							j.Err = "Failed to remove SSH key"
						}
						t.done <- j

					case dogeboxd.AddBinaryCache:
						err := t.AddBinaryCache(a, j.Logger.Step("Add binary cache"))
						if err != nil {
							j.Err = "Failed to add binary cache"
						}
						t.done <- j

					case dogeboxd.RemoveBinaryCache:
						err := t.removeBinaryCache(a)
						if err != nil {
							j.Err = "Failed to remove binary cache"
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
	t.jobs <- j
}

func (t SystemUpdater) GetUpdateChannel() chan dogeboxd.Job {
	return t.done
}

func (t SystemUpdater) markPupBroken(s dogeboxd.PupState, reason string, upstreamError error) error {
	_, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupBrokenReason(reason), dogeboxd.SetPupInstallation(dogeboxd.STATE_BROKEN))
	if err != nil {
		log.Printf("Failed to even mark pup as broken after issue: %w", err)
		return err
	}

	log.Printf("Marked pup %s as broken because: %s", s.ID, reason)

	return upstreamError
}

/* InstallPup takes a PupManifest and ensures a nix config
 * is written and any packages installed so that the Pup can
 * be started.
 */
func (t SystemUpdater) installPup(pupSelection dogeboxd.InstallPup, j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("install")
	nixPatch := t.nix.NewPatch(log)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_INSTALLING)); err != nil {
		log.Errf("Failed to update pup installation state: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	log.Logf("Installing pup from %s: %s @ %s", pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)

	log.Logf("Downloading pup to %s", pupPath)
	err := t.sources.DownloadPup(pupPath, pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	if err != nil {
		log.Errf("Failed to download pup: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// Ensure the nix file configured in the manifest matches the hash specified.
	// Read pupPath s.Manifest.Container.Build.NixFile and hash it with sha256
	nixFile, err := os.ReadFile(filepath.Join(pupPath, s.Manifest.Container.Build.NixFile))
	if err != nil {
		log.Errf("Failed to read specified nix file: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_FILE_MISSING, err)
	}
	nixFileSha256 := sha256.Sum256(nixFile)

	// Compare the sha256 hash of the nix file to the hash specified in the manifest
	if fmt.Sprintf("%x", nixFileSha256) != s.Manifest.Container.Build.NixFileSha256 {
		log.Errf("Nix file hash mismatch")
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_HASH_MISMATCH, err)
	}

	// create the storage dir
	cmd := exec.Command("sudo", "_dbxroot", "pup", "create-storage", "--data-dir", t.config.DataDir, "--pupId", s.ID)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		log.Errf("Failed to create pup storage: %v. Command output: %s", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// write delegate key to storage dir
	keyData, err := t.dkm.MakeDelegate(s.ID, pupSelection.SessionToken)
	if err != nil {
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_CREATION_FAILED, err)
	}

	cmd = exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", s.ID, "--key-file", "delegated.key", "--data", keyData.Priv)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		log.Errf("Failed to create delegate key in storage: %v. Command output: %s", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED, err)
	}

	cmd = exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", s.ID, "--key-file", "delegated.extended.key", "--data", keyData.Wif)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		log.Errf("Failed to create extended delegate key in storage: %v. Command output: %s", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED, err)
	}

	// Now that we're mostly installed, enable it.
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Errf("Failed to update pup enabled state: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_ENABLE_FAILED, err)
	}

	dbxState := t.sm.Get().Dogebox

	t.nix.WritePupFile(nixPatch, newState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	// Do a nix rebuild before we mark the pup as installed, this way
	// the frontend will get a much longer "Installing.." state, as opposed
	// to a much longer "Starting.." state, which might confuse the user.
	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Errf("Failed to update pup installation state: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	return nil
}

func (t SystemUpdater) uninstallPup(j dogeboxd.Job) error {
	// TODO: uninstall deps if they're not needed by another pup.
	s := *j.State
	log := j.Logger.Step("uninstall")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Uninstalling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLING)); err != nil {
		log.Errf("Failed to update pup uninstalling state: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	t.nix.RemovePupFile(nixPatch, s.ID)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLED)); err != nil {
		log.Errf("Failed to update pup installation state: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	return nil
}

func (t SystemUpdater) purgePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("purge")
	// Check if we're in a purgable state before we do anything.
	if s.Installation != dogeboxd.STATE_UNINSTALLED {
		log.Errf("Cannot purge pup %s in state %s", s.ID, s.Installation)
		return fmt.Errorf("Cannot purge pup %s in state %s", s.ID, s.Installation)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_PURGING)); err != nil {
		log.Errf("Failed to update pup purging state: %w", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	pupDir := filepath.Join(t.config.DataDir, "pups")

	log.Logf("Purging pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	// Delete pup state from disk
	if err := os.Remove(filepath.Join(pupDir, fmt.Sprintf("pup_%s.gob", s.ID))); err != nil {
		log.Errf("Failed to remove pup state %v", err)
		// Keep going if we fail.
	}

	// Delete downloaded pup source
	if err := os.RemoveAll(filepath.Join(pupDir, s.ID)); err != nil {
		log.Errf("Failed to remove pup source %v", err)
		// Keep going if we fail.
	}

	// Delete pup storage directory
	cmd := exec.Command("sudo", "_dbxroot", "pup", "delete-storage", "--pupId", s.ID, "--data-dir", t.config.DataDir)
	log.LogCmd(cmd)

	if err := cmd.Run(); err != nil {
		log.Errf("Failed to remove pup storage: %v", err)
		// Keep going if we fail.
	}

	if err := t.pupManager.PurgePup(s.ID); err != nil {
		log.Errf("Failed to purge pup %s: %w", s.ID, err)
		// Keep going if we fail.
	}

	return nil
}

func (t SystemUpdater) enablePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("enable")
	log.Logf("Enabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Errf("Failed to update pup enabled state: %w", err)
		return err
	}
	log.Log("set pup state to enabled")

	dbxState := t.sm.Get().Dogebox

	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %w", err)
		return err
	}

	return nil
}

func (t SystemUpdater) disablePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("disable")
	log.Logf("Disabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(false))
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
	log.LogCmd(cmd)

	if err := cmd.Run(); err != nil {
		log.Errf("Error executing _dbxroot pup stop:", err)
		return err
	}

	dbxState := t.sm.Get().Dogebox

	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %w", err)
		return err
	}

	return nil
}

func (t SystemUpdater) AddBinaryCache(j dogeboxd.AddBinaryCache, log dogeboxd.SubLogger) error {
	dbxState := t.sm.Get().Dogebox

	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return fmt.Errorf("failed to generate random ID for binary cache: %v", err)
	}

	dbxState.BinaryCaches = append(dbxState.BinaryCaches, dogeboxd.DogeboxStateBinaryCache{
		ID:   string(id),
		Host: j.Host,
		Key:  j.Key,
	})

	if err := t.sm.SetDogebox(dbxState); err != nil {
		return err
	}

	nixPatch := t.nix.NewPatch(log)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(nixPatch, values)

	return nixPatch.Apply()
}

func (t SystemUpdater) removeBinaryCache(j dogeboxd.RemoveBinaryCache) error {
	dbxState := t.sm.Get().Dogebox

	keyFound := false
	for i, cache := range dbxState.BinaryCaches {
		if cache.ID == j.ID {
			dbxState.BinaryCaches = append(dbxState.BinaryCaches[:i], dbxState.BinaryCaches[i+1:]...)
			keyFound = true
			break
		}
	}

	if !keyFound {
		return fmt.Errorf("binary cache with ID %s not found", j.ID)
	}

	return t.sm.SetDogebox(dbxState)
}
