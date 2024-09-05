package system

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	dbus "github.com/coreos/go-systemd/v22/dbus"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/shirou/gopsutil/v4/process"
)

func NewSystemMonitor(config dogeboxd.ServerConfig) SystemMonitor {
	services := []string{}
	return SystemMonitor{
		config:   config,
		services: services,
		mon:      make(chan []string, 10),
		stats:    make(chan map[string]dogeboxd.ProcStatus),
	}
}

/* SystemMonitor
 *
 * SystemMonitor accepts arrays of strings contianing
 * Systemd service names, ie: 'dogecoind.service' via
 * it's 'mon' channel. These are then observed every N
 * seconds and the monitor issues []dogeboxd.ProcStatus
 * results on the 'stats' channel.
 *
 * send a service name beginning with '-' to remove a
 * service from the monitoring list.
 *
 * Sending a service (again) will cause the SystemMonitor
 * to respond immediately with ProcStatus for those services.
 */

type SystemMonitor struct {
	config   dogeboxd.ServerConfig
	services []string
	mon      chan []string
	stats    chan map[string]dogeboxd.ProcStatus
}

func (t SystemMonitor) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
		mainloop:
			for {
				select {
				case <-stop:
					break mainloop
				case s := <-t.mon:
					t.updateServices(s)
				case <-timer.C:
					stats, err := getStatus(t.services)
					if err != nil {
						fmt.Println("error getting stats from systemd:", err)
						continue mainloop
					}
					select {
					case t.stats <- stats:
					default:
						fmt.Println("couldn't write to output channel")
					}
					timer.Reset(5 * time.Second)
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

func (t *SystemMonitor) updateServices(args []string) {
	t.services = args
}

func getStatus(serviceNames []string) (map[string]dogeboxd.ProcStatus, error) {
	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	out := map[string]dogeboxd.ProcStatus{}
	for _, service := range serviceNames {
		pidProp, err := conn.GetServicePropertyContext(context.Background(), service, "MainPID")
		if err != nil {
			continue
		}
		pid := pidProp.Value.Value().(uint32)
		cpu := float64(0)
		mem := float64(0)
		rssM := float64(0)
		running := false

		proc, err := process.NewProcess(int32(pid))
		if err == nil {
			running = true

			c, err := proc.CPUPercent()
			if err == nil {
				cpu = c
			}

			m, err := proc.MemoryPercent()
			if err == nil {
				mem = float64(m)
			}

			memInfo, err := proc.MemoryInfo()
			if err == nil {
				rssM = float64(memInfo.RSS) / float64(1048576)
			}

		}

		out[service] = dogeboxd.ProcStatus{
			CPUPercent: cpu,
			MEMPercent: mem,
			MEMMb:      rssM,
			Running:    running,
		}
	}

	return out, err
}

func (t SystemMonitor) GetMonChannel() chan []string {
	return t.mon
}

func (t SystemMonitor) GetStatChannel() chan map[string]dogeboxd.ProcStatus {
	return t.stats
}
