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
SystemUpdater implements dogeboxd.Jobber

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig) SystemUpdater {
	return SystemUpdater{
		config: config,
		jobs:   make(chan dogeboxd.Job),
		done:   make(chan dogeboxd.Job),
	}
}

type SystemUpdater struct {
	config dogeboxd.ServerConfig
	jobs   chan dogeboxd.Job
	done   chan dogeboxd.Job
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
				case v, ok := <-t.jobs:
					if !ok {
						break dance
					}
					switch a := v.A.(type) {
					case dogeboxd.InstallPup:
						err := installPup(t.config.NixDir, a.M)
						if err != nil {
							fmt.Println("Failed to install pup", err)
							v.Err = "Failed to install pup"
						}
						t.done <- v
					case dogeboxd.UninstallPup:
						// TODO
						t.done <- v
					case dogeboxd.StartPup:
						// TODO
						t.done <- v
					case dogeboxd.StopPup:
						// TODO
						t.done <- v
					case dogeboxd.RestartPup:
						// TODO
						t.done <- v
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
}

/* InstallPup takes a PupManifest and ensures a nix config
 * is written and any packages installed so that the Pup can
 * be started.
 *
 *
 */
func installPup(nixConfPath string, m dogeboxd.PupManifest) error {
	// write the nix config

	v := nixTemplateValues{
		SERVICE_NAME: m.Package,
		EXEC_COMMAND: m.Command.Path,
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
