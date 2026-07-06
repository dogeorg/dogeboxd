package system

import (
	"context"
	"fmt"
	"testing"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testNixManager struct {
	calls   []string
	blockOn string
}

func (t *testNixManager) InitSystem(patch dogeboxd.NixPatch, dbxState dogeboxd.DogeboxState) {}

func (t *testNixManager) UpdateIncludesFile(patch dogeboxd.NixPatch, pups dogeboxd.PupManager) {}

func (t *testNixManager) WritePupFile(patch dogeboxd.NixPatch, state dogeboxd.PupState, dbxState dogeboxd.DogeboxState) {}

func (t *testNixManager) RemovePupFile(patch dogeboxd.NixPatch, pupID string) {}

func (t *testNixManager) UpdateSystemContainerConfiguration(patch dogeboxd.NixPatch) {}

func (t *testNixManager) UpdateFirewallRules(patch dogeboxd.NixPatch, dbxState dogeboxd.DogeboxState) {}

func (t *testNixManager) UpdateNetwork(patch dogeboxd.NixPatch, values dogeboxd.NixNetworkTemplateValues) {}

func (t *testNixManager) UpdateSystem(patch dogeboxd.NixPatch, values dogeboxd.NixSystemTemplateValues) {}

func (t *testNixManager) UpdateStorageOverlay(patch dogeboxd.NixPatch, partitionName string) {}

func (t *testNixManager) RebuildBoot(log dogeboxd.SubLogger) error { return nil }

func (t *testNixManager) Rebuild(log dogeboxd.SubLogger) error { return nil }

func (t *testNixManager) NewPatch(log dogeboxd.SubLogger) dogeboxd.NixPatch { return nil }

func (t *testNixManager) GetConfigValue(configItem string) (string, error) {
	return t.GetConfigValueContext(context.Background(), configItem)
}

func (t *testNixManager) GetConfigValueContext(ctx context.Context, configItem string) (string, error) {
	t.calls = append(t.calls, configItem)
	if configItem == t.blockOn {
		<-ctx.Done()
		return "", fmt.Errorf("timed out waiting for nix config %q: %w", configItem, ctx.Err())
	}

	return configItem, nil
}

func TestUpdateNixCacheWarmsExpectedConfigSections(t *testing.T) {
	updater := SystemUpdater{
		nix: &testNixManager{},
	}

	err := updater.updateNixCache(testNixCacheJob())

	require.NoError(t, err)
	assert.Equal(t, []string{"console", "time"}, updater.nix.(*testNixManager).calls)
}

func TestUpdateNixCacheTimesOut(t *testing.T) {
	originalTimeout := nixCacheUpdateTimeout
	nixCacheUpdateTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		nixCacheUpdateTimeout = originalTimeout
	})

	fakeNix := &testNixManager{blockOn: "console"}
	updater := SystemUpdater{
		nix: fakeNix,
	}

	err := updater.updateNixCache(testNixCacheJob())

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, []string{"console"}, fakeNix.calls)
}

func testNixCacheJob() dogeboxd.Job {
	job := dogeboxd.Job{
		ID: "job-nix-cache",
		A:  dogeboxd.UpdateNixCache{},
	}
	job.Logger = dogeboxd.NewActionLogger(job, "", dogeboxd.Dogeboxd{
		Changes: make(chan dogeboxd.Change, 8),
	})

	return job
}
