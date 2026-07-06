package pup

import "testing"

func TestNextAvailablePortsReturnsUniquePortsInSingleAllocation(t *testing.T) {
	manager := PupManager{}

	ports := manager.nextAvailablePorts(2)
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	if ports[0] == ports[1] {
		t.Fatalf("expected unique ports, got duplicate %d", ports[0])
	}
}
