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

//go:embed templates/pup_container.nix
var rawPupContainerTemplate []byte

//go:embed templates/system_container_config.nix
var rawSystemContainerConfigTemplate []byte

//go:embed templates/firewall.nix
var rawFirewallTemplate []byte

//go:embed templates/system.nix
var rawSystemTemplate []byte

//go:embed templates/dogebox.nix
var rawIncludesFileTemplate []byte

//go:embed templates/network.nix
var rawNetworkTemplate []byte

var _ dogeboxd.NixManager = &nixManager{}

type nixManager struct {
	config dogeboxd.ServerConfig
}

func NewNixManager(config dogeboxd.ServerConfig) dogeboxd.NixManager {
	return nixManager{
		config: config,
	}
}

func (nm nixManager) InitSystem(pups dogeboxd.PupManager) error {
	if err := nm.UpdateIncludeFile(pups); err != nil {
		return err
	}

	// TODO: set these values properly
	sshEnabled := false
	hostIp := "10.69.0.1"
	containerCidr := "10.69.0.0/8"
	sshKeys := []string{}
	systemHostname := "dogebox"

	if err := nm.UpdateSystem(dogeboxd.NixSystemTemplateValues{
		SSH_ENABLED:     sshEnabled,
		SSH_KEYS:        sshKeys,
		SYSTEM_HOSTNAME: systemHostname,
	}); err != nil {
		return err
	}

	if err := nm.UpdateFirewall(dogeboxd.NixFirewallTemplateValues{
		SSH_ENABLED: sshEnabled,
	}); err != nil {
		return err
	}

	if err := nm.UpdateSystemContainerConfiguration(dogeboxd.NixSystemContainerConfigTemplateValues{
		// NETWORK_INTERFACE:      networkInterface,
		DOGEBOX_HOST_IP:        hostIp,
		DOGEBOX_CONTAINER_CIDR: containerCidr,
	}); err != nil {
		return err
	}

	return nil
}

func (nm nixManager) UpdateIncludeFile(pups dogeboxd.PupManager) error {
	installed := pups.GetStateMap()
	var pupIDs []string
	for id, state := range installed {
		if state.Installation == dogeboxd.STATE_READY || state.Installation == dogeboxd.STATE_RUNNING {
			pupIDs = append(pupIDs, id)
		}
	}

	values := dogeboxd.NixIncludesFileTemplateValues{
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
	services := []dogeboxd.NixPupContainerServiceValues{}

	for _, service := range state.Manifest.Container.Services {
		cwd := filepath.Join(fmt.Sprintf("${pkgs.pup.%s}", service.Name), service.Command.CWD)

		services = append(services, dogeboxd.NixPupContainerServiceValues{
			NAME: service.Name,
			EXEC: service.Command.Exec,
			CWD:  cwd,
			ENV:  convertEnvMapToSlice(service.Command.ENV),
		})
	}

	values := dogeboxd.NixPupContainerTemplateValues{
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

func (nm nixManager) UpdateSystemContainerConfiguration(values dogeboxd.NixSystemContainerConfigTemplateValues) error {
	return nm.writeTemplate("system_container_config.nix", rawSystemContainerConfigTemplate, values)
}

func (nm nixManager) UpdateFirewall(values dogeboxd.NixFirewallTemplateValues) error {
	return nm.writeTemplate("firewall.nix", rawFirewallTemplate, values)
}

func (nm nixManager) UpdateSystem(values dogeboxd.NixSystemTemplateValues) error {
	return nm.writeTemplate("system.nix", rawSystemTemplate, values)
}

func (nm nixManager) UpdateNetwork(values dogeboxd.NixNetworkTemplateValues) error {
	return nm.writeTemplate("network.nix", rawNetworkTemplate, values)
}

func (nm nixManager) RebuildBoot() error {
	// This command is setuid as root so we can actually run it.
	// It should live in /run/wrappers/bin/nixosrebuildboot on nix systems.
	md := exec.Command("nixosrebuildboot")

	output, err := md.CombinedOutput()
	if err != nil {
		log.Printf("Error executing nix rebuild boot: %v\n", err)
		log.Printf("nix output: %s\n", string(output))
		return err
	} else {
		log.Printf("nix output: %s\n", string(output))
	}
	return nil
}

func (nm nixManager) Rebuild() error {
	// This command is setuid as root so we can actually run it.
	// It should live in /run/wrappers/bin/nixosrebuildswitch on nix systems.
	md := exec.Command("nixosrebuildswitch")

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
