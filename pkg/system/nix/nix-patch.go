package nix

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
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

type NixPatch struct {
	nm          nixManager
	snapshotDir string
	state       string
	operations  []string
	error       error
}

func (np *NixPatch) State() string {
	return np.state
}

func (np *NixPatch) add(operation string) error {
	if np.state != NixPatchStatePending {
		return errors.New("patch already applied or cancelled")
	}

	np.operations = append(np.operations, operation)
	return nil
}

func (np *NixPatch) Apply() error {
	if np.state != NixPatchStatePending {
		return errors.New("patch already applied or cancelled")
	}

	np.state = NixPatchStateApplying

	if err := np.snapshot(); err != nil {
		np.state = NixPatchStateErrored
		np.error = err
		return fmt.Errorf("failed to snapshot: %w", err)
	}

	np.state = NixPatchStateApplying

	for _, operation := range np.operations {
		log.Printf("applying operation: %s", operation)
		// if operationApplyError != nil {
		// 	return np.triggerRollback(operationApplyError)
		// }
	}

	if err := np.nm.Rebuild(); err != nil {
		// We failed.
		// Roll back our changes.
		return np.triggerRollback(err)
	}

	np.state = NixPatchStateApplied

	return nil
}

func (np *NixPatch) Cancel() error {
	if np.state != NixPatchStatePending {
		return errors.New("patch already applied or cancelled")
	}

	np.state = NixPatchStateCancelled
	return nil
}

func (np *NixPatch) snapshot() error {
	timestamp := time.Now().Unix()

	snapshotDir := filepath.Join(np.nm.config.TmpDir, fmt.Sprintf("nix-patch-%d", timestamp))
	err := os.MkdirAll(snapshotDir, 0750)
	if err != nil {
		np.state = NixPatchStateErrored
		np.error = err
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	np.snapshotDir = snapshotDir
	return np.copyDirectory(np.nm.config.NixDir, np.snapshotDir)
}

func (np *NixPatch) triggerRollback(err error) error {
	np.state = NixPatchStateRollingBack
	np.error = err

	if err := np.doRollback(); err != nil {
		return fmt.Errorf("failed to actually roll back: %w", err)
	}

	np.state = NixPatchStateErrored
	return err
}

func (np *NixPatch) doRollback() error {
	if np.state != NixPatchStateApplying {
		return nil
	}

	np.state = NixPatchStateRollingBack

	err := os.RemoveAll(np.nm.config.NixDir)
	if err != nil {
		return fmt.Errorf("failed to remove nixDir: %w", err)
	}

	return np.copyDirectory(np.snapshotDir, np.nm.config.NixDir)
}

func (np *NixPatch) copyDirectory(srcDir, destDir string) error {
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
