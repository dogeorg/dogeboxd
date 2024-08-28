package nix

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
)

type NixManager interface {
	Rebuild() error
	WriteDogeboxNixFile(filename string, content string) error
}

//go:embed templates/pup_container.nix
var rawPupContainerTemplate []byte

type PupContainerTemplateValues struct {
	PUP_SLUG    string
	PUP_ENABLED bool
	INTERNAL_IP string
	PUP_PORTS   []int
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

//go:embed templates/overlays.nix
var rawOverlaysTemplate []byte

type OverlayTemplateValues struct {
	PUPS []struct {
		PUP_NAME string
		PUP_PATH string
	}
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

func (nm nixManager) WriteDogeboxNixFile(filename string, content string) error {
	fullPath := filepath.Join(nm.config.NixDir, filename)

	err := os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}

	return nil
}

func (nm nixManager) writeTemplate(filename string, _template []byte, values interface{}) error {
	template, err := template.New("firewall").Parse(string(_template))
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

// TODO; this shouldn't be defined here.
type PupConfiguration struct {
	ContainerIP string
}

func (nm nixManager) WritePupFiles(
	pupManifest pup.PupManifest,
	pupConfiguration PupConfiguration,
) error {
	values := PupContainerTemplateValues{
		PUP_SLUG:    pupManifest.Meta.Name,
		PUP_ENABLED: true,
		INTERNAL_IP: pupConfiguration.ContainerIP,
		PUP_PORTS:   []int{},
	}

	for _, ex := range pupManifest.Container.Exposes {
		values.PUP_PORTS = append(values.PUP_PORTS, ex.Port)
	}

	filename := fmt.Sprintf("pup_%s.nix", pupManifest.Meta.Name)

	return nm.writeTemplate(filename, rawPupContainerTemplate, values)
}

func (nm nixManager) WriteSystemContainerConfiguration(values SystemContainerConfigTemplateValues) error {
	return nm.writeTemplate("system_container_config.nix", rawSystemContainerConfigTemplate, values)
}

func (nm nixManager) UpdateFirewall(values FirewallTemplateValues) error {
	return nm.writeTemplate("firewall.nix", rawFirewallTemplate, values)
}

func (nm nixManager) UpdateSystem(values SystemTemplateValues) error {
	return nm.writeTemplate("system.nix", rawSystemTemplate, values)
}

func (nm nixManager) UpdateOverlays(values OverlayTemplateValues) error {
	return nm.writeTemplate("overlays.nix", rawOverlaysTemplate, values)
}

func (nm nixManager) Rebuild() error {
	md := exec.Command("nixos-rebuild", "switch")

	output, err := md.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing nix rebuild: %s\n", err)
		return err
	} else {
		fmt.Printf("nix output: %s\n", string(output))
	}
	return nil
}
