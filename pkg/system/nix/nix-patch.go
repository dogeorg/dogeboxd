package nix

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"text/template"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

//go:embed templates/pup_container.nix
var rawPupContainerTemplate []byte

//go:embed templates/system_container_config.nix
var rawSystemContainerConfigTemplate []byte

//go:embed templates/firewall.nix
var rawFirewallTemplate []byte

//go:embed templates/system.nix
var rawSystemTemplate []byte

//go:embed templates/dogebox.nix
var rawIncludesFileTemplate []byte

//go:embed templates/network.nix
var rawNetworkTemplate []byte

const (
	NixPatchStatePending     string = "pending"
	NixPatchStateCancelled   string = "cancelled"
	NixPatchStateApplying    string = "applying"
	NixPatchStateApplied     string = "applied"
	NixPatchStateRollingBack string = "rolling back"
	NixPatchStateErrored     string = "errored"
)

var _ dogeboxd.NixPatch = &nixPatch{}

type PatchOperation struct {
	Name      string
	Operation func() error
}

type nixPatch struct {
	id          string
	nm          nixManager
	snapshotDir string
	state       string
	operations  []PatchOperation
	error       error
}

func NewNixPatch(nm nixManager) dogeboxd.NixPatch {
	id := make([]byte, 6)
	rand.Read(id)
	patchID := hex.EncodeToString(id)

	p := &nixPatch{
		id:    patchID,
		nm:    nm,
		state: NixPatchStatePending,
	}

	log.Printf("[patch-%s] Created new nix patch", p.id)

	return p
}

func (np *nixPatch) State() string {
	return np.state
}

func (np *nixPatch) add(name string, op func() error) error {
	if np.state != NixPatchStatePending {
		return errors.New("patch already applied or cancelled")
	}

	log.Printf("[patch-%s] Adding pending operation %s", np.id, name)
	np.operations = append(np.operations, PatchOperation{Name: name, Operation: op})

	return nil
}

func (np *nixPatch) Apply() error {
	return np.ApplyCustom(dogeboxd.NixPatchApplyOptions{})
}

func (np *nixPatch) ApplyCustom(options dogeboxd.NixPatchApplyOptions) error {
	if np.state != NixPatchStatePending {
		return errors.New("patch already applied or cancelled")
	}

	log.Printf("[patch-%s] Applying nix patch with %d operations", np.id, len(np.operations))

	np.state = NixPatchStateApplying

	if err := np.snapshot(); err != nil {
		np.state = NixPatchStateErrored
		np.error = err
		return fmt.Errorf("failed to snapshot: %w", err)
	}

	np.state = NixPatchStateApplying

	for _, operation := range np.operations {
		log.Printf("[patch-%s] Applying operation: %s", np.id, operation.Name)
		if err := operation.Operation(); err != nil {
			return np.triggerRollback(err)
		}
	}

	log.Printf("[patch-%s] Applied all patch operations, rebuilding..", np.id)

	var rebuildFn func() error

	if options.RebuildBoot {
		rebuildFn = np.nm.RebuildBoot
	} else {
		rebuildFn = np.nm.Rebuild
	}

	if err := rebuildFn(); err != nil {
		// We failed.
		// Roll back our changes.
		log.Printf("[patch-%s] Failed to rebuild, rolling back.. %v", np.id, err)
		return np.triggerRollback(err)
	}

	if err := os.RemoveAll(np.snapshotDir); err != nil {
		log.Printf("[patch-%s] Warning: Failed to remove snapshot directory: %v", np.id, err)
	} else {
		log.Printf("[patch-%s] Removed snapshot directory: %s", np.id, np.snapshotDir)
	}

	np.state = NixPatchStateApplied
	log.Printf("[patch-%s] Nix patch applied successfully", np.id)

	return nil
}

func (np *nixPatch) Cancel() error {
	if np.state != NixPatchStatePending {
		return errors.New("patch already applied or cancelled")
	}

	np.state = NixPatchStateCancelled
	return nil
}

func (np *nixPatch) snapshot() error {
	timestamp := time.Now().Unix()

	snapshotDir := filepath.Join(np.nm.config.TmpDir, fmt.Sprintf("nix-patch-%d", timestamp))
	err := os.MkdirAll(snapshotDir, 0750)
	if err != nil {
		np.state = NixPatchStateErrored
		np.error = err
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	log.Printf("[patch-%s] Snapshotting nix directory to %s", np.id, snapshotDir)

	np.snapshotDir = snapshotDir
	return copyDirectory(np.nm.config.NixDir, np.snapshotDir)
}

func (np *nixPatch) triggerRollback(err error) error {
	log.Printf("[patch-%s] Triggering rollback", np.id)
	log.Printf("[patch-%s] Rollback triggered because of error: %v", np.id, err)

	np.state = NixPatchStateRollingBack
	np.error = err

	if err := np.doRollback(); err != nil {
		log.Printf("[patch-%s] Failed to actually roll back: %v", np.id, err)
		return fmt.Errorf("failed to actually roll back: %w", err)
	}

	np.state = NixPatchStateErrored
	return err
}

func (np *nixPatch) doRollback() error {
	if np.state != NixPatchStateApplying {
		return nil
	}

	np.state = NixPatchStateRollingBack

	err := os.RemoveAll(np.nm.config.NixDir)
	if err != nil {
		return fmt.Errorf("failed to remove nixDir: %w", err)
	}

	return copyDirectory(np.snapshotDir, np.nm.config.NixDir)
}

func (np *nixPatch) UpdateSystemContainerConfiguration(values dogeboxd.NixSystemContainerConfigTemplateValues) {
	np.add("UpdateSystemContainerConfiguration", func() error {
		return np.writeTemplate("system_container_config.nix", rawSystemContainerConfigTemplate, values)
	})
}

func (np *nixPatch) UpdateFirewall(values dogeboxd.NixFirewallTemplateValues) {
	np.add("UpdateFirewall", func() error {
		return np.writeTemplate("firewall.nix", rawFirewallTemplate, values)
	})
}

func (np *nixPatch) UpdateSystem(values dogeboxd.NixSystemTemplateValues) {
	np.add("UpdateSystem", func() error {
		return np.writeTemplate("system.nix", rawSystemTemplate, values)
	})
}

func (np *nixPatch) UpdateNetwork(values dogeboxd.NixNetworkTemplateValues) {
	np.add("UpdateNetwork", func() error {
		return np.writeTemplate("network.nix", rawNetworkTemplate, values)
	})
}

func (np *nixPatch) UpdateIncludesFile(values dogeboxd.NixIncludesFileTemplateValues) {
	np.add("UpdateIncludesFile", func() error {
		return np.writeTemplate("dogebox.nix", rawIncludesFileTemplate, values)
	})
}

func (np *nixPatch) WritePupFile(pupId string, values dogeboxd.NixPupContainerTemplateValues) {
	np.add("WritePupFile", func() error {
		filename := fmt.Sprintf("pup_%s.nix", pupId)
		return np.writeTemplate(filename, rawPupContainerTemplate, values)
	})
}

func (np *nixPatch) writeTemplate(filename string, _template []byte, values interface{}) error {
	template, err := template.New(filename).Parse(string(_template))
	if err != nil {
		return err
	}

	var contents bytes.Buffer
	err = template.Execute(&contents, values)
	if err != nil {
		return err
	}

	err = np.writeDogeboxNixFile(filename, contents.String())
	if err != nil {
		return err
	}

	return nil
}

func (np *nixPatch) RemovePupFile(pupId string) {
	np.add("RemovePupFile", func() error {
		// Remove pup nix file
		filename := fmt.Sprintf("pup_%s.nix", pupId)
		if _, err := os.Stat(filepath.Join(np.nm.config.NixDir, filename)); err == nil {
			if err := os.Remove(filepath.Join(np.nm.config.NixDir, filename)); err != nil {
				return fmt.Errorf("failed to remove file %s: %w", filename, err)
			}
		}
		return nil
	})
}

func (np *nixPatch) writeDogeboxNixFile(filename string, content string) error {
	fullPath := filepath.Join(np.nm.config.NixDir, filename)

	err := os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directories for %s: %w", fullPath, err)
	}
	err = os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}

	return nil
}

func copyDirectory(srcDir, destDir string) error {
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}

		return os.Chmod(destPath, info.Mode())
	})

	if err != nil {
		return fmt.Errorf("failed to copy files: %w", err)
	}

	return nil
}
