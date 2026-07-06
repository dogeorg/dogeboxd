package system

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func GetCustomNixPath(config dogeboxd.ServerConfig) string {
	return filepath.Join(config.DataDir, "custom.nix")
}

func getLegacyCustomNixPath(config dogeboxd.ServerConfig) string {
	return filepath.Join(config.NixDir, "custom.nix")
}

func hashFile(path string) ([sha256.Size]byte, error) {
	var hash [sha256.Size]byte

	file, err := os.Open(path)
	if err != nil {
		return hash, err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return hash, err
	}

	copy(hash[:], hasher.Sum(nil))
	return hash, nil
}

func moveFileWithVerification(sourcePath, destinationPath string) error {
	sourceHash, err := hashFile(sourcePath)
	if err != nil {
		return err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, sourceInfo.Mode().Perm())
	if err != nil {
		return err
	}

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		destinationFile.Close()
		_ = os.Remove(destinationPath)
		return err
	}

	if err := destinationFile.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return err
	}

	destinationHash, err := hashFile(destinationPath)
	if err != nil {
		_ = os.Remove(destinationPath)
		return err
	}
	if sourceHash != destinationHash {
		_ = os.Remove(destinationPath)
		return errors.New("copied custom.nix hash mismatch")
	}

	return os.Remove(sourcePath)
}

func MigrateLegacyCustomNix(config dogeboxd.ServerConfig) error {
	customNixPath := GetCustomNixPath(config)
	legacyCustomNixPath := getLegacyCustomNixPath(config)

	// If the new persistent file already exists, there is nothing to migrate.
	if _, err := os.Stat(customNixPath); err == nil {
		return nil
	}

	// If the legacy file does not exist either, there is also nothing to migrate.
	// Any other stat error should still stop the request.
	if _, err := os.Stat(legacyCustomNixPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	// Ensure the persistent parent directory exists before copying the file.
	if err := os.MkdirAll(filepath.Dir(customNixPath), 0755); err != nil {
		return err
	}

	// Copy the legacy file into its persistent location, verify the copied
	// contents match, and only then remove the old file.
	return moveFileWithVerification(legacyCustomNixPath, customNixPath)
}

// ValidateNix validates nix content using nix-instantiate --parse.
// Returns nil if valid, otherwise returns the error message.
func (t SystemUpdater) ValidateNix(content string) error {
	// Create a temporary file to validate
	tmpFile, err := os.CreateTemp(t.config.TmpDir, "custom-nix-validate-*.nix")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Run nix-instantiate --parse to validate syntax
	cmd := exec.Command("nix-instantiate", "--parse", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &NixValidationError{Output: string(output)}
	}

	return nil
}

// NixValidationError represents a nix validation error
type NixValidationError struct {
	Output string
}

func (e *NixValidationError) Error() string {
	return e.Output
}

// SaveCustomNix validates and saves the custom.nix content,
// then triggers a system rebuild.
func (t SystemUpdater) SaveCustomNix(content string, l dogeboxd.SubLogger) error {
	l.Logf("Validating custom nix configuration...")

	// Validate first
	if err := t.ValidateNix(content); err != nil {
		l.Errf("Validation failed: %v", err)
		return err
	}

	l.Logf("Validation passed, saving configuration...")

	// Save the file
	customNixPath := GetCustomNixPath(t.config)
	if err := os.MkdirAll(filepath.Dir(customNixPath), 0755); err != nil {
		l.Errf("Failed to create custom.nix directory: %v", err)
		return err
	}
	if err := os.WriteFile(customNixPath, []byte(content), 0644); err != nil {
		l.Errf("Failed to write custom.nix: %v", err)
		return err
	}

	// dogebox.nix now imports the data-dir custom.nix directly via the template,
	// so saving the file is enough before triggering a rebuild.
	l.Logf("Triggering system rebuild...")

	// Trigger rebuild
	if err := t.nix.Rebuild(l); err != nil {
		l.Errf("Rebuild failed: %v", err)
		return err
	}

	l.Logf("Custom configuration applied successfully")
	return nil
}
