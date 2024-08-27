package system

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager) SystemUpdater {
	return SystemUpdater{
		config:  config,
		jobs:    make(chan dogeboxd.Job),
		done:    make(chan dogeboxd.Job),
		network: networkManager,
	}
}

type SystemUpdater struct {
	config  dogeboxd.ServerConfig
	jobs    chan dogeboxd.Job
	done    chan dogeboxd.Job
	network dogeboxd.NetworkManager
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

//go:embed template.nix
var nixTemplate []byte

type nixTemplateValues struct {
	SERVICE_NAME string
	EXEC_COMMAND string
	IP           string
	ENABLED      bool
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
	m := s.Manifest
	v := nixTemplateValues{
		SERVICE_NAME: m.ID,
		EXEC_COMMAND: m.Command.Path,
		IP:           s.IP,
		ENABLED:      enabled,
	}
	t, err := template.New("nixService").Parse(string(nixTemplate))
	if err != nil {
		fmt.Println("Failed to parse template.nix")
		return err
	}
	// write the template to the nixConfPath
	p := filepath.Join(nixConfPath, fmt.Sprintf("pup_%s.nix", v.SERVICE_NAME))
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println("Failed to open nixConfigPath for writing", v.SERVICE_NAME)
		return err
	}
	defer f.Close()

	err = t.Execute(f, v)
	if err != nil {
		fmt.Println("Failed to write template to nixPath", v.SERVICE_NAME)
		return err
	}

	// rebuild the nix system
	err = nixRebuild()
	if err != nil {
		return err
	}
	return nil
}

func deleteNix(nixConfPath string, s dogeboxd.PupState) error {
	m := s.Manifest
	p := filepath.Join(nixConfPath, fmt.Sprintf("pup_%s.nix", m.ID))
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
