package system

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type SystemUpdater struct {
	jobs dogeboxd.Job
}

/* CAVEAT:  Many of these functions are blocking, and are
* designed to be executed within a goroutine.
 */

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
func InstallPup(nixConfPath string, m dogeboxd.PupManifest) error {
	// write the nix config

	v := nixTemplateValues{
		SERVICE_NAME: m.Package,
		EXEC_COMAND:  m.Path,
	}

	// get the template from the cache or cache it
	t := template.Lookup("nixService")
	if template == nil {
		t, err := template.New("nixService").Parse(nixTemplate)
		if err != nil {
			fmt.Println("Failed to parse template.nix")
			return err
		}
	}

	// write the template to the nixConfPath
	p := filepath.Join(nixConfPath, fmt.sprintf("pup_%s.nix", v.SERVICE_NAME))
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println("Failed to open nixConfigPath for writing", v.SERVICE_NAME)
		return err
	}
	defer f.Close()

	err := t.Execute(f, v)
	if err != nil {
		fmt.Println("Failed to write template to nixPath", v.SERVICE_NAME)
		return err
	}

	// rebuild the nix system
	err := nixRebuild()
	if err {
		return err
	}
}

func nixRebuild() err {
	md := exec.Command("nix", "rebuild")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing nix rebuild: %s\n", err)
		return err
	} else {
		fmt.Printf("nix output: %s\n", string(output))
	}
	return nil
}
