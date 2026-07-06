package system

import (
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestIsInstallTargetDisk(t *testing.T) {
	tests := []struct {
		name string
		disk dogeboxd.SystemDisk
		want bool
	}{
		{
			name: "allows ordinary install target disks",
			disk: dogeboxd.SystemDisk{
				Name: "/dev/nvme0n1",
				Suitability: dogeboxd.SystemDiskSuitability{
					Install: dogeboxd.SystemDiskSuitabilityEntry{
						Usable: true,
						SizeOK: true,
					},
				},
			},
			want: true,
		},
		{
			name: "rejects the boot media disk",
			disk: dogeboxd.SystemDisk{
				Name:      "/dev/mmcblk0",
				BootMedia: true,
				Suitability: dogeboxd.SystemDiskSuitability{
					Install: dogeboxd.SystemDiskSuitabilityEntry{
						Usable: true,
						SizeOK: true,
					},
				},
			},
			want: false,
		},
		{
			name: "rejects disks that do not meet the minimum install size",
			disk: dogeboxd.SystemDisk{
				Name: "/dev/mmcblk2boot0",
				Suitability: dogeboxd.SystemDiskSuitability{
					Install: dogeboxd.SystemDiskSuitabilityEntry{
						Usable: true,
						SizeOK: false,
					},
				},
			},
			want: false,
		},
		{
			name: "rejects disks that fail existing install suitability checks",
			disk: dogeboxd.SystemDisk{
				Name: "/dev/sda",
				Suitability: dogeboxd.SystemDiskSuitability{
					Install: dogeboxd.SystemDiskSuitabilityEntry{
						Usable: true,
						SizeOK: false,
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInstallTargetDisk(tt.disk); got != tt.want {
				t.Fatalf("IsInstallTargetDisk(%q) = %v, want %v", tt.disk.Name, got, tt.want)
			}
		})
	}
}
