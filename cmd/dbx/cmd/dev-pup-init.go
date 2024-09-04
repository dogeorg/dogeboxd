package cmd

import (
	_ "embed"
	"os"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

//go:embed templates/manifest.template.json
var manifestTemplate []byte

//go:embed templates/pup.template.nix
var pupTemplate []byte

//go:embed templates/server.template.go
var serverGoTemplate []byte

type TemplateValues struct {
	PUP_NAME string
}

// initCmd represents the init command
var pupInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new pup",
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")

		if name == "" {
			cmd.PrintErrln("Error: pup name is required")
			return
		}

		cwd, err := os.Getwd()
		if err != nil {
			cmd.PrintErrln("Error: failed to get current working directory")
			return
		}
		pupDir := filepath.Join(cwd, name)
		if _, err := os.Stat(pupDir); !os.IsNotExist(err) {
			cmd.PrintErrln("Error: pup directory already exists")
			return
		}

		if err := os.MkdirAll(pupDir, 0755); err != nil {
			cmd.PrintErrln("Error: failed to create pup directory")
			return
		}

		values := TemplateValues{
			PUP_NAME: name,
		}

		if err := writeTemplate(filepath.Join(pupDir, "manifest.json"), manifestTemplate, values); err != nil {
			cmd.PrintErrln("Error: failed to write manifest template")
			return
		}

		if err := writeTemplate(filepath.Join(pupDir, "pup.nix"), pupTemplate, values); err != nil {
			cmd.PrintErrln("Error: failed to write pup template")
			return
		}

		if err := writeTemplate(filepath.Join(pupDir, "server.go"), serverGoTemplate, values); err != nil {
			cmd.PrintErrln("Error: failed to write server template")
			return
		}

		cmd.Println("Pup initialized successfully:", pupDir)
	},
}

func writeTemplate(path string, templateRaw []byte, data any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	tmpl, err := template.New(path).Parse(string(templateRaw))
	if err != nil {
		return err
	}

	err = tmpl.Execute(file, data)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	pupInitCmd.Flags().StringP("name", "n", "", "Name of the pup")
	pupInitCmd.MarkFlagRequired("name")
	pupCmd.AddCommand(pupInitCmd)
}
