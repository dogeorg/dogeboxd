package system

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t SystemUpdater) sshUpdate(dbxState dogeboxd.DogeboxState, log dogeboxd.SubLogger) error {
	patch := t.nix.NewPatch(log)
	t.nix.UpdateFirewallRules(patch, dbxState)
	t.nix.UpdateSystem(patch, dogeboxd.NixSystemTemplateValues{
		SYSTEM_HOSTNAME: dbxState.Hostname,
		SSH_ENABLED:     dbxState.SSH.Enabled,
		SSH_KEYS:        dbxState.SSH.Keys,
	})

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to enable SSH: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) EnableSSH(l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox
	state.SSH.Enabled = true
	t.sm.SetDogebox(state)
	if err := t.sm.Save(); err != nil {
		return err
	}

	return t.sshUpdate(state, l)
}

func (t SystemUpdater) DisableSSH(l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox
	state.SSH.Enabled = false
	t.sm.SetDogebox(state)
	if err := t.sm.Save(); err != nil {
		return err
	}

	return t.sshUpdate(state, l)
}

func (t SystemUpdater) ListSSHKeys() ([]dogeboxd.DogeboxStateSSHKey, error) {
	state := t.sm.Get().Dogebox
	return state.SSH.Keys, nil
}

func (t SystemUpdater) AddSSHKey(key string, l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox

	keyID := make([]byte, 8)
	if _, err := rand.Read(keyID); err != nil {
		return fmt.Errorf("failed to generate random key ID: %v", err)
	}

	state.SSH.Keys = append(state.SSH.Keys, dogeboxd.DogeboxStateSSHKey{
		ID:  hex.EncodeToString(keyID),
		Key: key,
	})

	t.sm.SetDogebox(state)
	if err := t.sm.Save(); err != nil {
		return err
	}

	return t.sshUpdate(state, l)
}

func (t SystemUpdater) RemoveSSHKey(id string, l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox

	keyFound := false
	for i, key := range state.SSH.Keys {
		if key.ID == id {
			state.SSH.Keys = append(state.SSH.Keys[:i], state.SSH.Keys[i+1:]...)
			keyFound = true
			break
		}
	}

	if !keyFound {
		return fmt.Errorf("SSH key with ID %s not found", id)
	}

	t.sm.SetDogebox(state)
	if err := t.sm.Save(); err != nil {
		return err
	}

	return t.sshUpdate(state, l)
}
