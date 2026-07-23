//go:build linux

package lifecycle

import (
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestLinuxLifecycleUsesRootWrapperDirectly(t *testing.T) {
	original := runRootCommand
	t.Cleanup(func() { runRootCommand = original })

	var actions []string
	runRootCommand = func(action string) error {
		actions = append(actions, action)
		return nil
	}
	manager := LifecycleManagerLinux{config: dogeboxd.ServerConfig{}}

	manager.Reboot()
	manager.Shutdown()

	if len(actions) != 2 || actions[0] != "reboot" || actions[1] != "shutdown" {
		t.Fatalf("unexpected root-wrapper actions: %v", actions)
	}
}
