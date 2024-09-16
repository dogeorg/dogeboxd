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
	pups   dogeboxd.PupManager
}

func NewNixManager(config dogeboxd.ServerConfig, pups dogeboxd.PupManager) dogeboxd.NixManager {
	return nixManager{
		config: config,
		pups:   pups,
	}
}

func (nm nixManager) InitSystem() error {
	if err := nm.UpdateIncludeFile(nm.pups); err != nil {
		return err
	}

	// TODO: set these values properly
	sshEnabled := false
	sshKeys := []string{}
	systemHostname := "dogebox"

	if err := nm.UpdateSystem(dogeboxd.NixSystemTemplateValues{
		SSH_ENABLED:     sshEnabled,
		SSH_KEYS:        sshKeys,
		SYSTEM_HOSTNAME: systemHostname,
	}); err != nil {
		return err
	}

	if err := nm.UpdateFirewallRules(); err != nil {
		return err
	}

	if err := nm.UpdateSystemContainerConfiguration(); err != nil {
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
		PUP_ID:      state.ID,
		PUP_ENABLED: state.Enabled,
		INTERNAL_IP: state.IP,
		PUP_PORTS: []struct {
			PORT   int
			PUBLIC bool
		}{},
		STORAGE_PATH: filepath.Join(nm.config.DataDir, "pups/storage", state.ID),
		PUP_PATH:     filepath.Join(nm.config.DataDir, "pups", state.ID),
		NIX_FILE:     filepath.Join(nm.config.DataDir, "pups", state.ID, state.Manifest.Container.Build.NixFile),
		SERVICES:     services,
	}

	hasPublicPorts := false

	for _, ex := range state.Manifest.Container.Exposes {
		values.PUP_PORTS = append(values.PUP_PORTS, struct {
			PORT   int
			PUBLIC bool
		}{
			PORT:   ex.Port,
			PUBLIC: ex.ListenOnHost,
		})

		if ex.ListenOnHost {
			hasPublicPorts = true
		}
	}

	// If we have any public host ports, we need to
	// update the host firewall to open those ports.
	if hasPublicPorts {
		if err := nm.UpdateFirewallRules(); err != nil {
			return err
		}
	}

	// If we need access to the internet, update the system container config.
	if state.Manifest.Container.RequiresInternet {
		if err := nm.UpdateSystemContainerConfiguration(); err != nil {
			return err
		}
	}

	filename := fmt.Sprintf("pup_%s.nix", state.ID)

	return nm.writeTemplate(filename, rawPupContainerTemplate, values)
}

func (nm nixManager) RemovePupFile(pupId string) error {
	// Remove pup nix file
	filename := fmt.Sprintf("pup_%s.nix", pupId)
	return os.Remove(filepath.Join(nm.config.NixDir, filename))
}

func (nm nixManager) UpdateSystemContainerConfiguration() error {
	// TODO: Move away from hardcoding these values. Should be pulled from pupmanager?
	hostIp := "10.69.0.1"
	containerCidr := "10.69.0.0/8"

	pupState := nm.pups.GetStateMap()
	var pupsRequiringInternet []dogeboxd.NixSystemContainerConfigTemplatePupRequiresInternet
	for _, state := range pupState {
		if state.Manifest.Container.RequiresInternet {
			pupsRequiringInternet = append(pupsRequiringInternet, dogeboxd.NixSystemContainerConfigTemplatePupRequiresInternet{
				PUP_ID: state.ID,
				PUP_IP: state.IP,
			})
		}
	}

	var pupsTcpConnections []dogeboxd.NixSystemContainerConfigTemplatePupTcpConnection
	for _, state := range pupState {
		for _, dependency := range state.Manifest.Dependencies {
			// TODO: Do this.
		}
	}

	values := dogeboxd.NixSystemContainerConfigTemplateValues{
		DOGEBOX_HOST_IP:         hostIp,
		DOGEBOX_CONTAINER_CIDR:  containerCidr,
		PUPS_REQUIRING_INTERNET: pupsRequiringInternet,
		PUPS_TCP_CONNECTIONS:    pupsTcpConnections,
	}

	return nm.updateSystemContainerConfiguration(values)
}

func (nm nixManager) updateSystemContainerConfiguration(values dogeboxd.NixSystemContainerConfigTemplateValues) error {
	return nm.writeTemplate("system_container_config.nix", rawSystemContainerConfigTemplate, values)
}

func (nm nixManager) UpdateFirewallRules() error {
	installed := nm.pups.GetStateMap()
	var pupPorts []struct {
		PORT   int
		PUBLIC bool
		PUP_ID string
	}

	for pupID, state := range installed {
		for _, port := range state.Manifest.Container.Exposes {
			pupPorts = append(pupPorts, struct {
				PORT   int
				PUBLIC bool
				PUP_ID string
			}{
				PORT:   port.Port,
				PUBLIC: port.ListenOnHost,
				PUP_ID: pupID,
			})
		}
	}

	// TODO: set these values properly
	sshEnabled := false

	return nm.updateFirewall(dogeboxd.NixFirewallTemplateValues{
		SSH_ENABLED: sshEnabled,
		PUP_PORTS:   pupPorts,
	})
}

func (nm nixManager) updateFirewall(values dogeboxd.NixFirewallTemplateValues) error {
	return nm.writeTemplate("firewall.nix", rawFirewallTemplate, values)
}

func (nm nixManager) UpdateSystem(values dogeboxd.NixSystemTemplateValues) error {
	return nm.writeTemplate("system.nix", rawSystemTemplate, values)
}

func (nm nixManager) UpdateNetwork(values dogeboxd.NixNetworkTemplateValues) error {
	return nm.writeTemplate("network.nix", rawNetworkTemplate, values)
}

func (nm nixManager) RebuildBoot() error {
	md := exec.Command("sudo", "_dbxroot", "nix", "rb")

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
	md := exec.Command("sudo", "_dbxroot", "nix", "rs")

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
