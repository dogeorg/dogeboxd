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
	"strings"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/utils"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager, nixManager dogeboxd.NixManager, sourceManager dogeboxd.SourceManager, pupManager dogeboxd.PupManager, stateManager dogeboxd.StateManager, lifecycle dogeboxd.LifecycleManager, dkm dogeboxd.DKMManager) SystemUpdater {
	return SystemUpdater{
		config:     config,
		jobs:       make(chan dogeboxd.Job),
		done:       make(chan dogeboxd.Job),
		network:    networkManager,
		nix:        nixManager,
		sources:    sourceManager,
		pupManager: pupManager,
		sm:         stateManager,
		lifecycle:  lifecycle,
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
	lifecycle  dogeboxd.LifecycleManager
	dkm        dogeboxd.DKMManager
}

var nixCacheUpdateTimeout = 60 * time.Second

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
					case dogeboxd.UpgradePup:
						err := t.upgradePup(a, j)
						if err != nil {
							j.Err = "Failed to upgrade pup"
						}
						t.done <- j
					case dogeboxd.RollbackPupUpgrade:
						err := t.rollbackPupUpgrade(j)
						if err != nil {
							j.Err = "Failed to rollback pup"
						}
						t.done <- j
					case dogeboxd.ImportBlockchainData:
						err := t.importBlockchainData(j)
						if err != nil {
							j.Err = "Failed to import blockchain data"
						}
						t.done <- j
					case dogeboxd.UpdatePendingSystemNetwork:
						err := t.network.SetPendingNetwork(a.Network, j)
						if err != nil {
							j.Err = "Failed to set system network"
						}
						t.done <- j

					case dogeboxd.InitialBootstrap:
						err := t.initialBootstrap(a, j)
						if err != nil {
							j.Err = err.Error()
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

					case dogeboxd.SaveCustomNix:
						err := t.SaveCustomNix(a.Content, j.Logger.Step("save custom nix"))
						if err != nil {
							j.Err = "Failed to save custom configuration"
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

					case dogeboxd.SystemUpdate:
						logger := j.Logger.Step("system update")
						logger.Progress(5).Logf("Starting system update to %s", a.Version)
						if err := t.DoSystemUpdate(a.Package, a.Version, logger); err != nil {
							logger.Errf("System update failed: %v", err)
							j.Err = err.Error()
						} else {
							logger.Progress(100).Logf("System update to %s completed", a.Version)
						}
						t.done <- j

					case dogeboxd.UpdateTimezone:
						err := t.updateTimezone(a, j.Logger.Step("update timezone"))
						if err != nil {
							j.Err = "Failed to update timezone"
						}
						t.done <- j

					case dogeboxd.UpdateKeymap:
						err := t.updateKeymap(a, j.Logger.Step("update keymap"))
						if err != nil {
							j.Err = "Failed to update keyboard layout"
						}
						t.done <- j

					case dogeboxd.UpdateNixCache:
						err := t.updateNixCache(j)
						if err != nil {
							j.Err = err.Error()
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

// HasSnapshot checks if a snapshot exists for a pup (for rollback)
func (t SystemUpdater) HasSnapshot(pupID string) bool {
	return t.pupManager.HasSnapshot(pupID)
}

// GetSnapshot retrieves a snapshot for a pup if it exists
func (t SystemUpdater) GetSnapshot(pupID string) (*dogeboxd.PupVersionSnapshot, error) {
	return t.pupManager.GetSnapshot(pupID)
}

func (t SystemUpdater) markPupBroken(s dogeboxd.PupState, reason string, upstreamError error) error {
	_, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupBrokenReason(reason), dogeboxd.SetPupInstallation(dogeboxd.STATE_BROKEN))
	if err != nil {
		log.Printf("Failed to even mark pup as broken after issue: %v", err)
		return err
	}

	log.Printf("Marked pup %s as broken because: %s", s.ID, reason)

	return upstreamError
}

// cleanupSidebarPreferences removes a pup from sidebar preferences after uninstall/purge
func (t SystemUpdater) cleanupSidebarPreferences(pupID string) {
	dbxState := t.sm.Get().Dogebox
	if dbxState.SidebarPups == nil {
		return // Nothing to clean up
	}

	// Remove pupID from list
	filtered := []string{}
	for _, id := range dbxState.SidebarPups {
		if id != pupID {
			filtered = append(filtered, id)
		}
	}

	dbxState.SidebarPups = filtered
	t.sm.SetDogebox(dbxState)
}

// verifyNixFileHash verifies that the nix file matches the expected hash from the manifest
func (t SystemUpdater) verifyNixFileHash(pupPath string, manifest dogeboxd.PupManifest, isDevMode bool, logger dogeboxd.SubLogger) error {
	nixFile, err := os.ReadFile(filepath.Join(pupPath, manifest.Container.Build.NixFile))
	if err != nil {
		return fmt.Errorf("failed to read nix file: %w", err)
	}

	nixFileSha256 := sha256.Sum256(nixFile)
	actualHash := fmt.Sprintf("%x", nixFileSha256)

	if actualHash != manifest.Container.Build.NixFileSha256 {
		logger.Errf("Nix file hash mismatch! Manifest Hash: %s, Computed Hash: %s", manifest.Container.Build.NixFileSha256, actualHash)
		if !isDevMode {
			return fmt.Errorf("nix file hash mismatch")
		}
		logger.Log("Warning: Nix hash mismatch ignored in dev mode")
	}

	return nil
}

/* InstallPup takes a PupManifest and ensures a nix config
 * is written and any packages installed so that the Pup can
 * be started.
 */
func (t SystemUpdater) installPup(pupSelection dogeboxd.InstallPup, j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("install")

	log.Logf("Installing pup: name=%s, version=%s, manifestVersion=%s",
		s.Manifest.Meta.Name, s.Version, s.Manifest.Meta.Version)
	nixPatch := t.nix.NewPatch(log)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_INSTALLING)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	log.Logf("Installing pup from %s: %s @ %s", pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)

	log.Logf("Downloading pup to %s", pupPath)
	downloadedManifest, err := t.sources.DownloadPup(pupPath, pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	if err != nil {
		log.Errf("Failed to download pup: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// Verify nix file hash using the downloaded manifest
	if err := t.verifyNixFileHash(pupPath, downloadedManifest, s.IsDevModeEnabled, log); err != nil {
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_HASH_MISMATCH, err)
	}

	// create the storage dir
	cmd := exec.Command("sudo", "_dbxroot", "pup", "create-storage", "--data-dir", t.config.DataDir, "--pupId", s.ID)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO : Do we need command output here?
		log.Errf("Failed to create pup storage: %v", err)
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
		// TODO : Do we need command output here?
		log.Errf("Failed to create delegate key in storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED, err)
	}

	cmd = exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", s.ID, "--key-file", "delegated.extended.key", "--data", keyData.Wif)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO : Do we need command output here?
		log.Errf("Failed to create extended delegate key in storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED, err)
	}

	// Write initial config to secure storage (includes defaults from manifest)
	// This ensures config.env exists before the container starts
	if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, s.ID, s.Config, log); err != nil {
		log.Errf("Failed to write initial config to storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// Now that we're mostly installed, enable it.
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Errf("Failed to update pup enabled state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_ENABLE_FAILED, err)
	}

	dbxState := t.sm.Get().Dogebox

	t.nix.WritePupFile(nixPatch, newState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	// Do a nix rebuild before we mark the pup as installed, this way
	// the frontend will get a much longer "Installing.." state, as opposed
	// to a much longer "Starting.." state, which might confuse the user.
	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	log.Logf("Pup installation complete: pupID=%s, version=%s, name=%s", s.ID, s.Version, s.Manifest.Meta.Name)

	return nil
}

func (t SystemUpdater) uninstallPup(j dogeboxd.Job) error {
	// TODO: uninstall deps if they're not needed by another pup.
	s := *j.State
	log := j.Logger.Step("uninstall")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Uninstalling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if _, err := t.pupManager.UpdatePup(
		s.ID,
		dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLING),
		dogeboxd.PupEnabled(false),
	); err != nil {
		log.Errf("Failed to update pup uninstalling state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	t.nix.RemovePupFile(nixPatch, s.ID)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLED)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// Clean up sidebar preferences
	t.cleanupSidebarPreferences(s.ID)

	return nil
}

func (t SystemUpdater) purgePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("purge")
	// Check if we're in a purgable state before we do anything.
	if s.Installation != dogeboxd.STATE_UNINSTALLED {
		log.Errf("Cannot purge pup %s in state %s", s.ID, s.Installation)
		return fmt.Errorf("cannot purge pup %s in state %s", s.ID, s.Installation)
	}

	if _, err := t.pupManager.UpdatePup(
		s.ID,
		dogeboxd.SetPupInstallation(dogeboxd.STATE_PURGING),
		dogeboxd.PupEnabled(false),
	); err != nil {
		log.Errf("Failed to update pup purging state: %v", err)
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
		log.Errf("Failed to purge pup %s: %v", s.ID, err)
		// Keep going if we fail.
	}

	// Clean up sidebar preferences
	t.cleanupSidebarPreferences(s.ID)

	return nil
}

func (t SystemUpdater) enablePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("enable")

	// Enabled flag should already be set by dispatcher, but verify/set for idempotency
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Errf("Failed to update pup enabled state: %v", err)
		return err
	}

	dbxState := t.sm.Get().Dogebox

	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) disablePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("disable")

	// Enabled flag should already be set to false by dispatcher, but verify/set for idempotency
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(false))
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
	log.LogCmd(cmd)

	if err := cmd.Run(); err != nil {
		log.Errf("Error executing _dbxroot pup stop: %v", err)
		return err
	}

	dbxState := t.sm.Get().Dogebox

	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) importBlockchainData(j dogeboxd.Job) error {
	log := j.Logger.Step("import-blockchain-data")
	log.Log("Starting blockchain data import")

	// Find the Dogecoin Core pup
	var dogecoinPup *dogeboxd.PupState
	pupStateMap := t.pupManager.GetStateMap()
	for _, pup := range pupStateMap {
		if pup.Manifest.Meta.Name == "Dogecoin Core" {
			dogecoinPup = &pup
			break
		}
	}

	var wasEnabled bool
	if dogecoinPup != nil {
		log.Logf("Found Dogecoin Core pup: %s (ID: %s)", dogecoinPup.Manifest.Meta.Name, dogecoinPup.ID)
		wasEnabled = dogecoinPup.Enabled

		// If the pup is enabled, disable it to prevent auto-restart during import
		if wasEnabled {
			log.Log("Dogecoin Core pup is enabled, temporarily disabling during import...")
			_, err := t.pupManager.UpdatePup(dogecoinPup.ID, dogeboxd.PupEnabled(false))
			if err != nil {
				log.Errf("Failed to disable pup: %v", err)
				return err
			}

			// Stop the pup if it's running
			stopCmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", dogecoinPup.ID)
			log.LogCmd(stopCmd)
			if err := stopCmd.Run(); err != nil {
				log.Errf("Error stopping pup: %v", err)
				// Re-enable the pup if we failed to stop it
				t.pupManager.UpdatePup(dogecoinPup.ID, dogeboxd.PupEnabled(true))
				return err
			}
		}
	}

	// Run the blockchain data import command
	cmd := exec.Command("sudo", "_dbxroot", "import-blockchain-data", "--data-dir", t.config.DataDir)
	log.LogCmd(cmd)

	err := cmd.Run()
	if err != nil {
		log.Errf("Failed to import blockchain data: %v", err)
	}

	// Re-enable the pup if it was originally enabled
	if dogecoinPup != nil && wasEnabled {
		log.Log("Re-enabling Dogecoin Core pup...")
		_, enableErr := t.pupManager.UpdatePup(dogecoinPup.ID, dogeboxd.PupEnabled(true))
		if enableErr != nil {
			log.Errf("Failed to re-enable pup: %v", enableErr)
			if err == nil {
				err = enableErr
			}
		} else {
			// Apply nix patch to ensure the pup configuration is updated
			dbxState := t.sm.Get().Dogebox
			nixPatch := t.nix.NewPatch(log)
			pupState, _, pupErr := t.pupManager.GetPup(dogecoinPup.ID)
			if pupErr == nil {
				t.nix.WritePupFile(nixPatch, pupState, dbxState)
				if applyErr := nixPatch.Apply(); applyErr != nil {
					log.Errf("Failed to apply nix patch: %v", applyErr)
				}
			}
		}
	}

	if err != nil {
		return err
	}

	log.Log("Blockchain data import completed")
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
		}
	}

	if !keyFound {
		return fmt.Errorf("binary cache with ID %s not found", j.ID)
	}

	return t.sm.SetDogebox(dbxState)
}

func (t SystemUpdater) UpdateSystemConfig(dbxState dogeboxd.DogeboxState, log dogeboxd.SubLogger) error {
	patch := t.nix.NewPatch(log)
	t.nix.UpdateFirewallRules(patch, dbxState)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(patch, values)

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to commit system state: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) updateTimezone(a dogeboxd.UpdateTimezone, log dogeboxd.SubLogger) error {
	log.Logf("Updating timezone to %s", a.Timezone)

	dbxState := t.sm.Get().Dogebox
	dbxState.Timezone = a.Timezone

	if err := t.sm.SetDogebox(dbxState); err != nil {
		log.Errf("Failed to save timezone state: %v", err)
		return err
	}

	log.Progress(20).Log("Applying system configuration...")

	patch := t.nix.NewPatch(log)
	t.nix.UpdateFirewallRules(patch, dbxState)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(patch, values)

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return err
	}

	log.Progress(100).Logf("Timezone updated to %s", a.Timezone)
	return nil
}

func (t SystemUpdater) updateKeymap(a dogeboxd.UpdateKeymap, log dogeboxd.SubLogger) error {
	log.Logf("Updating keyboard layout to %s", a.Keymap)

	dbxState := t.sm.Get().Dogebox
	dbxState.KeyMap = a.Keymap

	if err := t.sm.SetDogebox(dbxState); err != nil {
		log.Errf("Failed to save keymap state: %v", err)
		return err
	}

	log.Progress(20).Log("Applying system configuration...")

	patch := t.nix.NewPatch(log)
	t.nix.UpdateFirewallRules(patch, dbxState)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(patch, values)

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return err
	}

	log.Progress(100).Logf("Keyboard layout updated to %s", a.Keymap)
	return nil
}

func (t SystemUpdater) updateNixCache(j dogeboxd.Job) error {
	log := j.Logger.Step("update nix cache")
	log.Log("Updating nix cache...")
	ctx, cancel := context.WithTimeout(context.Background(), nixCacheUpdateTimeout)
	defer cancel()

	// These two sections should be a balance between pre-populating what
	// we need and not eating too many resources to fetch.
	if _, err := t.nix.GetConfigValueContext(ctx, "console"); err != nil {
		log.Errf("Failed to get console section from nix config: %v", err)
		return err
	}

	if _, err := t.nix.GetConfigValueContext(ctx, "time"); err != nil {
		log.Errf("Failed to get time section from nix config: %v", err)
		return err
	}

	return nil
}

// getServiceStatus returns detailed status information about a systemd service
func getServiceStatus(serviceName string) (status string, recentLogs []string, err error) {
	// Get service status
	statusCmd := exec.Command("sudo", "systemctl", "status", serviceName, "--no-pager", "--lines=0")
	statusOutput, statusErr := statusCmd.CombinedOutput()
	status = strings.TrimSpace(string(statusOutput))

	// Get recent logs (last 20 lines)
	logsCmd := exec.Command("sudo", "journalctl", "-u", serviceName, "-n", "20", "--no-pager")
	logsOutput, logsErr := logsCmd.CombinedOutput()
	if logsErr == nil {
		logLines := strings.Split(strings.TrimSpace(string(logsOutput)), "\n")
		recentLogs = logLines
	}

	if statusErr != nil {
		// Service might not exist or be in failed state
		err = fmt.Errorf("service status check failed: %w", statusErr)
	}

	return status, recentLogs, err
}

// waitForContainerRunning polls systemctl until container is active and running
// This replaces manual systemctl start - we let NixOS autoStart handle it
func waitForContainerRunning(serviceName string, timeout time.Duration, log dogeboxd.SubLogger) error {
	deadline := time.Now().Add(timeout)
	checkInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		// Check if service is active and running
		cmd := exec.Command("sudo", "systemctl", "is-active", serviceName)
		output, _ := cmd.CombinedOutput()
		state := strings.TrimSpace(string(output))

		if state == "active" {
			// Double-check it's actually running (not just activated)
			cmd = exec.Command("sudo", "systemctl", "show", serviceName, "--property=SubState")
			output, _ = cmd.CombinedOutput()
			subState := strings.TrimSpace(strings.TrimPrefix(string(output), "SubState="))

			if subState == "running" {
				log.Logf("Container is active and running")
				return nil
			}

			log.Logf("Container state: %s (substate: %s), waiting...", state, subState)
		} else if state == "activating" {
			log.Logf("Container is activating, NixOS building system...")
		} else {
			log.Logf("Container state: %s, waiting for NixOS to build and start...", state)
		}

		time.Sleep(checkInterval)
	}

	return fmt.Errorf("timeout after %v waiting for container to start", timeout)
}

// upgradePup handles upgrading a pup to a new version while preserving config and data
func (t SystemUpdater) upgradePup(upgrade dogeboxd.UpgradePup, j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("upgrade")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Upgrading pup %s (%s) from %s to %s", s.Manifest.Meta.Name, s.ID, s.Version, upgrade.TargetVersion)

	// Record if pup was enabled
	wasEnabled := s.Enabled

	// Stop the pup if it's running
	if s.Enabled {
		log.Log("Stopping pup before upgrade...")
		if err := t.pupManager.StopPup(s.ID, t.nix, log); err != nil {
			log.Errf("Warning: failed to stop pup: %v", err)
			// Continue anyway, might not be running
		}
	}

	// Create a snapshot of current state for rollback
	log.Log("Creating snapshot for rollback...")
	if err := t.pupManager.CreateSnapshot(s); err != nil {
		log.Errf("Failed to create snapshot: %v", err)
		return fmt.Errorf("cannot proceed with upgrade without rollback capability: %w", err)
	}

	// Fetch the new manifest FIRST (before downloading files)
	// This allows us to update state before modifying files on disk
	log.Logf("Fetching manifest for version %s", upgrade.TargetVersion)
	newManifest, _, err := t.sources.GetSourceManifest(upgrade.SourceId, s.Manifest.Meta.Name, upgrade.TargetVersion)
	if err != nil {
		log.Errf("Failed to fetch manifest for target version: %v", err)
		return t.markPupBroken(s, "manifest_fetch_failed", err)
	}

	// Update state with new version/manifest BEFORE downloading files
	// This ensures state is always consistent - if download fails later,
	// we're in a broken state at the TARGET version (not old version with new files)
	// This mirrors the install flow where AdoptPup sets version before download
	_, err = t.pupManager.UpdatePup(s.ID,
		dogeboxd.SetPupInstallation(dogeboxd.STATE_UPGRADING),
		dogeboxd.SetPupVersion(upgrade.TargetVersion),
		dogeboxd.SetPupManifest(newManifest),
	)
	if err != nil {
		log.Errf("Failed to update pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// Clear the update cache entry now that version has changed
	// This ensures the "update available" badge disappears immediately
	t.pupManager.ClearCacheEntry(s.ID)

	// Now download files - state already reflects target version
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)
	log.Logf("Downloading new version to %s", pupPath)

	_, err = t.sources.DownloadPup(pupPath, upgrade.SourceId, s.Manifest.Meta.Name, upgrade.TargetVersion)
	if err != nil {
		log.Errf("Failed to download new version: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// Verify nix file hash
	if err := t.verifyNixFileHash(pupPath, newManifest, s.IsDevModeEnabled, log); err != nil {
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_HASH_MISMATCH, err)
	}

	// Write updated config to storage (in case manifest has new config fields)
	updatedState, _, err := t.pupManager.GetPup(s.ID)
	if err != nil {
		log.Errf("Failed to get updated pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, s.ID, updatedState.Config, log); err != nil {
		log.Errf("Failed to write config to storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// Rebuild nix configuration
	dbxState := t.sm.Get().Dogebox
	t.nix.WritePupFile(nixPatch, updatedState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	// For ephemeral containers, completely remove from NixOS config before re-adding
	// This forces NixOS to treat it as a NEW container and rebuild its system
	if wasEnabled {
		log.Log("Removing pup from NixOS config (will re-add as new)...")
		removeNixPatch := t.nix.NewPatch(log)
		removeNixPatch.RemovePupFile(s.ID)
		if err := removeNixPatch.Apply(); err != nil {
			log.Errf("Failed to remove pup from config: %v", err)
			return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
		}

		// Delete container directory to ensure clean slate
		containerDir := fmt.Sprintf("/var/lib/nixos-containers/pup-%s", s.ID)
		log.Logf("Cleaning container directory: %s", containerDir)
		if err := os.RemoveAll(containerDir); err != nil {
			log.Errf("Warning: failed to remove container directory: %v", err)
			// Not fatal, continue
		}
	}

	// Mark as ready and re-enable if it was enabled before
	updates := []func(*dogeboxd.PupState, *[]dogeboxd.Pupdate){dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)}
	if wasEnabled {
		log.Log("Re-enabling pup after upgrade...")
		updates = append(updates, dogeboxd.PupEnabled(true))
	}

	newState, err := t.pupManager.UpdatePup(s.ID, updates...)
	if err != nil {
		log.Errf("Failed to update pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// Write pup file with updated state (including Enabled=true if re-enabling) and rebuild
	// Since we removed it completely, NixOS will treat this as a NEW container
	if wasEnabled {
		log.Log("Adding pup back to NixOS config as new container...")
		nixPatch := t.nix.NewPatch(log)
		t.nix.WritePupFile(nixPatch, newState, dbxState)
		t.nix.UpdateIncludesFile(nixPatch, t.pupManager)
		if err := nixPatch.Apply(); err != nil {
			log.Errf("Failed to add pup back to config: %v", err)
			return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
		}

		// Container should start automatically via autoStart=true
		// NixOS will build the container system and start it because it's "new"
		serviceName := fmt.Sprintf("container@pup-%s.service", s.ID)
		log.Logf("Waiting for container to start (NixOS treating as new container)...")

		if err := waitForContainerRunning(serviceName, 60*time.Second, log); err != nil {
			log.Errf("Container did not start within timeout: %v", err)

			// Get detailed service status and logs for debugging
			status, logs, statusErr := getServiceStatus(serviceName)
			if statusErr != nil {
				log.Errf("Failed to get service status: %v", statusErr)
			} else {
				if status != "" {
					log.Logf("Service status:\n%s", status)
				}
				if len(logs) > 0 {
					log.Logf("Recent service logs (last %d lines):", len(logs))
					for _, logLine := range logs {
						log.Logf("  %s", logLine)
					}
				}
			}

			log.Errf("Check container logs: journalctl -u %s", serviceName)
			log.Errf("Container may still start in background, but upgrade workflow completing")
		} else {
			log.Logf("Container started successfully")
		}
	}

	log.Logf("Successfully upgraded pup %s to version %s", s.Manifest.Meta.Name, upgrade.TargetVersion)
	return nil
}

// rollbackPupUpgrade handles rolling back a pup to its previous version using a snapshot
func (t SystemUpdater) rollbackPupUpgrade(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("rollback")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Rolling back pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	snapshot, err := t.pupManager.GetSnapshot(s.ID)
	if err != nil {
		log.Errf("Failed to get snapshot: %v", err)
		return fmt.Errorf("failed to get snapshot: %w", err)
	}
	if snapshot == nil {
		log.Errf("No snapshot found for rollback")
		return fmt.Errorf("no snapshot available for rollback")
	}

	log.Logf("Found snapshot: rolling back to version %s", snapshot.Version)

	// Stop the pup if running
	cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
	log.LogCmd(cmd)
	_ = cmd.Run() // Ignore error, might not be running

	// Update state to indicate rollback in progress
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UPGRADING)); err != nil {
		log.Errf("Failed to update state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// Download the previous version
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)
	log.Logf("Downloading previous version %s", snapshot.Version)

	_, err = t.sources.DownloadPup(pupPath, snapshot.SourceID, s.Manifest.Meta.Name, snapshot.Version)
	if err != nil {
		log.Errf("Failed to download previous version: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// Restore state from snapshot (using manifest from snapshot, not downloaded one)
	_, err = t.pupManager.UpdatePup(s.ID,
		dogeboxd.SetPupVersion(snapshot.Version),
		dogeboxd.SetPupManifest(snapshot.Manifest),
		dogeboxd.SetPupConfig(snapshot.Config),
		dogeboxd.SetPupProviders(snapshot.Providers),
	)
	if err != nil {
		log.Errf("Failed to restore pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// Write config to storage
	if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, s.ID, snapshot.Config, log); err != nil {
		log.Errf("Failed to write config to storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// Mark as ready and re-enable if it was enabled before
	updates := []func(*dogeboxd.PupState, *[]dogeboxd.Pupdate){dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)}
	if snapshot.Enabled {
		log.Log("Re-enabling pup after rollback...")
		updates = append(updates, dogeboxd.PupEnabled(true))
	}

	if _, err := t.pupManager.UpdatePup(s.ID, updates...); err != nil {
		log.Errf("Failed to update pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// Rebuild nix configuration with all state updates (ready + enabled)
	restoredState, _, _ := t.pupManager.GetPup(s.ID)
	dbxState := t.sm.Get().Dogebox
	t.nix.WritePupFile(nixPatch, restoredState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	// Explicitly start the container if it was enabled - NixOS autoStart only starts
	// NEW containers, not containers that were previously stopped
	if snapshot.Enabled {
		serviceName := fmt.Sprintf("container@pup-%s.service", s.ID)
		cmd := exec.Command("sudo", "systemctl", "start", serviceName)
		log.LogCmd(cmd)
		if err := cmd.Run(); err != nil {
			log.Errf("Warning: failed to start container after rollback: %v", err)
			// Not fatal - container may start via other means
		}
	}

	// Delete the snapshot after successful rollback
	if err := t.pupManager.DeleteSnapshot(s.ID); err != nil {
		log.Errf("Warning: failed to delete snapshot: %v", err)
		// Not a fatal error
	}

	log.Logf("Successfully rolled back pup %s to version %s", s.Manifest.Meta.Name, snapshot.Version)
	return nil
}
