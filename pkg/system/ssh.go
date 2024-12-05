package system

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t SystemUpdater) sshUpdate(dbxState dogeboxd.DogeboxState, log dogeboxd.SubLogger) error {
	patch := t.nix.NewPatch(log)
	t.nix.UpdateFirewallRules(patch, dbxState)

	binaryCacheSubs := []string{}
	binaryCacheKeys := []string{}
	for _, cache := range dbxState.BinaryCaches {
		binaryCacheSubs = append(binaryCacheSubs, cache.Host)
		binaryCacheKeys = append(binaryCacheKeys, cache.Key)
	}

	t.nix.UpdateSystem(patch, dogeboxd.NixSystemTemplateValues{
		SYSTEM_HOSTNAME:   dbxState.Hostname,
		SSH_ENABLED:       dbxState.SSH.Enabled,
		SSH_KEYS:          dbxState.SSH.Keys,
		KEYMAP:            dbxState.KeyMap,
		BINARY_CACHE_SUBS: binaryCacheSubs,
		BINARY_CACHE_KEYS: binaryCacheKeys,
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

	if err := t.sm.SetDogebox(state); err != nil {
		return err
	}

	return t.sshUpdate(state, l)
}

func (t SystemUpdater) DisableSSH(l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox
	state.SSH.Enabled = false
	if err := t.sm.SetDogebox(state); err != nil {
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
		ID:        hex.EncodeToString(keyID),
		DateAdded: time.Now(),
		Key:       key,
	})

	if err := t.sm.SetDogebox(state); err != nil {
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

	if err := t.sm.SetDogebox(state); err != nil {
		return err
	}

	return t.sshUpdate(state, l)
}
