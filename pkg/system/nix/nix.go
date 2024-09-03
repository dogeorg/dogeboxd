package nix

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type NixManager interface {
	Rebuild() error
	Init(pups dogeboxd.PupManager) error
	UpdateIncludeFile(pups dogeboxd.PupManager) error
	WriteDogeboxNixFile(filename string, content string) error
	WritePupFile(pupState dogeboxd.PupState) error
	RemovePupFile(pupId string) error
	UpdateSystemContainerConfiguration(values SystemContainerConfigTemplateValues) error
}

//go:embed templates/pup_container.nix
var rawPupContainerTemplate []byte

type PupContainerServiceValues struct {
	NAME string
	EXEC string
	CWD  string
	ENV  []struct {
		KEY string
		VAL string
	}
}

type PupContainerTemplateValues struct {
	PUP_ID       string
	PUP_ENABLED  bool
	INTERNAL_IP  string
	PUP_PORTS    []int
	STORAGE_PATH string
	PUP_PATH     string
	NIX_FILE     string
	SERVICES     []PupContainerServiceValues
}

//go:embed templates/system_container_config.nix
var rawSystemContainerConfigTemplate []byte

type SystemContainerConfigTemplateValues struct {
	NETWORK_INTERFACE      string
	DOGEBOX_HOST_IP        string
	DOGEBOX_CONTAINER_CIDR string
}

//go:embed templates/firewall.nix
var rawFirewallTemplate []byte

type FirewallTemplateValues struct {
	SSH_ENABLED bool
}

//go:embed templates/system.nix
var rawSystemTemplate []byte

type SystemTemplateValues struct {
	SSH_ENABLED bool
	SSH_KEYS    []string
}

//go:embed templates/dogebox.nix
var rawIncludesFileTemplate []byte

type IncludesFileTemplateValues struct {
	PUP_IDS []string
}

var _ NixManager = &nixManager{}

type nixManager struct {
	config dogeboxd.ServerConfig
}

func NewNixManager(config dogeboxd.ServerConfig) NixManager {
	return nixManager{
		config: config,
	}
}

func (nm nixManager) Init(pups dogeboxd.PupManager) error {
	return nm.UpdateIncludeFile(pups)
}

func (nm nixManager) UpdateIncludeFile(pups dogeboxd.PupManager) error {
	installed := pups.GetStateMap()
	var pupIDs []string
	for id, state := range installed {
		if state.Installation == dogeboxd.STATE_READY || state.Installation == dogeboxd.STATE_RUNNING {
			pupIDs = append(pupIDs, id)
		}
	}

	values := IncludesFileTemplateValues{
		PUP_IDS: pupIDs,
	}

	return nm.writeTemplate("dogebox.nix", rawIncludesFileTemplate, values)
}

func (nm nixManager) WriteDogeboxNixFile(filename string, content string) error {
	fullPath := filepath.Join(nm.config.NixDir, filename)

	err := os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directories for %s: %w", fullPath, err)
	}
	err = os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}

	return nil
}

func (nm nixManager) writeTemplate(filename string, _template []byte, values interface{}) error {
	template, err := template.New(filename).Parse(string(_template))
	if err != nil {
		return err
	}

	var contents bytes.Buffer
	err = template.Execute(&contents, values)
	if err != nil {
		return err
	}

	err = nm.WriteDogeboxNixFile(filename, contents.String())
	if err != nil {
		return err
	}

	return nil
}

func (nm nixManager) WritePupFile(
	state dogeboxd.PupState,
) error {
	services := []PupContainerServiceValues{}

	for _, service := range state.Manifest.Container.Services {
		cwd := filepath.Join(fmt.Sprintf("${pkgs.pup.%s}", service.Name), service.Command.CWD)

		services = append(services, PupContainerServiceValues{
			NAME: service.Name,
			EXEC: service.Command.Exec,
			CWD:  cwd,
			ENV:  convertEnvMapToSlice(service.Command.ENV),
		})
	}

	values := PupContainerTemplateValues{
		PUP_ID:       state.ID,
		PUP_ENABLED:  state.Enabled,
		INTERNAL_IP:  state.IP,
		PUP_PORTS:    []int{},
		STORAGE_PATH: filepath.Join(nm.config.DataDir, "pups/storage", state.ID),
		PUP_PATH:     filepath.Join(nm.config.DataDir, "pups", state.ID),
		NIX_FILE:     filepath.Join(nm.config.DataDir, "pups", state.ID, state.Manifest.Container.Build.NixFile),
		SERVICES:     services,
	}

	for _, ex := range state.Manifest.Container.Exposes {
		values.PUP_PORTS = append(values.PUP_PORTS, ex.Port)
	}

	filename := fmt.Sprintf("pup_%s.nix", state.ID)

	return nm.writeTemplate(filename, rawPupContainerTemplate, values)
}

func (nm nixManager) RemovePupFile(pupId string) error {
	// Remove pup nix file
	filename := fmt.Sprintf("pup_%s.nix", pupId)
	return os.Remove(filepath.Join(nm.config.NixDir, filename))
}

func (nm nixManager) UpdateSystemContainerConfiguration(values SystemContainerConfigTemplateValues) error {
	return nm.writeTemplate("system_container_config.nix", rawSystemContainerConfigTemplate, values)
}

func (nm nixManager) UpdateFirewall(values FirewallTemplateValues) error {
	return nm.writeTemplate("firewall.nix", rawFirewallTemplate, values)
}

func (nm nixManager) UpdateSystem(values SystemTemplateValues) error {
	return nm.writeTemplate("system.nix", rawSystemTemplate, values)
}

func (nm nixManager) Rebuild() error {
	md := exec.Command("nixos-rebuild", "switch")

	output, err := md.CombinedOutput()
	if err != nil {
		log.Printf("Error executing nix rebuild: %v\n", err)
		log.Printf("nix output: %s\n", string(output))
		return err
	} else {
		log.Printf("nix output: %s\n", string(output))
	}
	return nil
}

func convertEnvMapToSlice(envMap map[string]string) []struct{ KEY, VAL string } {
	envSlice := make([]struct{ KEY, VAL string }, 0, len(envMap))
	for k, v := range envMap {
		envSlice = append(envSlice, struct{ KEY, VAL string }{KEY: k, VAL: v})
	}
	return envSlice
}
