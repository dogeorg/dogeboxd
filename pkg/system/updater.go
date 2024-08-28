package system

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system/nix"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager, nixManager nix.NixManager) SystemUpdater {
	return SystemUpdater{
		config:  config,
		jobs:    make(chan dogeboxd.Job),
		done:    make(chan dogeboxd.Job),
		network: networkManager,
		nix:     nixManager,
	}
}

type SystemUpdater struct {
	config  dogeboxd.ServerConfig
	jobs    chan dogeboxd.Job
	done    chan dogeboxd.Job
	network dogeboxd.NetworkManager
	nix     nix.NixManager
}

func (t SystemUpdater) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
			dance:
				select {
				case <-stop:
					break mainloop
				case j, ok := <-t.jobs:
					if !ok {
						break dance
					}
					switch a := j.A.(type) {
					case dogeboxd.InstallPup:
						fmt.Printf("JON %+v", a)
						err := installPup(t.config.NixDir, *j.State)
						if err != nil {
							fmt.Println("Failed to install pup", err)
							j.Err = "Failed to install pup"
						}
						t.done <- j
					case dogeboxd.UninstallPup:
						err := uninstallPup(t.config.NixDir, *j.State)
						if err != nil {
							fmt.Println("Failed to uninstall pup", err)
							j.Err = "Failed to uninstall pup"
						}
						t.done <- j
					case dogeboxd.EnablePup:
						err := enablePup(t.config.NixDir, *j.State)
						if err != nil {
							fmt.Println("Failed to enable pup", err)
							j.Err = "Failed to enable pup"
						}
						t.done <- j
					case dogeboxd.DisablePup:
						err := disablePup(t.config.NixDir, *j.State)
						if err != nil {
							fmt.Println("Failed to disable pup", err)
							j.Err = "Failed to disable pup"
						}
						t.done <- j
					case dogeboxd.UpdatePendingSystemNetwork:
						err := t.network.SetPendingNetwork(*&a.Network)
						if err != nil {
							fmt.Println("Failed to set system network:", err)
							j.Err = "Failed to set system network"
						}
						t.done <- j
					default:
						fmt.Printf("Unknown action type: %v\n", a)
					}
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

func (t SystemUpdater) AddJob(j dogeboxd.Job) {
	fmt.Printf("add job %+v\n", j)
	t.jobs <- j
}

func (t SystemUpdater) GetUpdateChannel() chan dogeboxd.Job {
	return t.done
}

/* InstallPup takes a PupManifest and ensures a nix config
 * is written and any packages installed so that the Pup can
 * be started.
 *
 *
 */
func installPup(nixConfPath string, s dogeboxd.PupState) error {
	// TODO: Install deps!
	return writeNix(true, nixConfPath, s)
}

func uninstallPup(nixConfPath string, s dogeboxd.PupState) error {
	// TODO: uninstall deps!
	return deleteNix(nixConfPath, s)
}

func enablePup(nixConfPath string, s dogeboxd.PupState) error {
	return writeNix(true, nixConfPath, s)
}

func disablePup(nixConfPath string, s dogeboxd.PupState) error {
	return writeNix(false, nixConfPath, s)
}

func writeNix(enabled bool, nixConfPath string, s dogeboxd.PupState) error {
	return nil
}

func deleteNix(nixConfPath string, s dogeboxd.PupState) error {
	m := s.Manifest
	p := filepath.Join(nixConfPath, fmt.Sprintf("pup_%s.nix", m.Meta.Name))
	err := os.Remove(p)
	if err != nil {
		return err
	}
	err = nixRebuild()
	if err != nil {
		return err
	}
	return nil
}

func nixRebuild() error {
	md := exec.Command("nix", "rebuild")

	output, err := md.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing nix rebuild: %s\n", err)
		return err
	} else {
		fmt.Printf("nix output: %s\n", string(output))
	}
	return nil
}
