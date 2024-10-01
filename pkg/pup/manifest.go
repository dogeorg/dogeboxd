package pup

import "fmt"

/* PupManifest represents a Nix installed process
 * running inside the Dogebox Runtime Environment.
 * These are defined in pup.json files.
 */
type PupManifest struct {
	// The version of the actual manifest. This differs from the "version"
	// of the pup, and the version of the deployed software for this pup.
	// Valid values: 1
	ManifestVersion int                     `json:"manifestVersion"`
	Meta            PupManifestMeta         `json:"meta"`
	Config          PupManifestConfigFields `json:"config"`
	Container       PupManifestContainer    `json:"container"`
	Interfaces      []PupManifestInterface  `json:"interfaces"`
	Dependencies    []PupManifestDependency `json:"dependencies"`
	Metrics         []PupManifestMetric     `json:"metrics"`
}

func (m *PupManifest) Validate() error {
	if m.ManifestVersion != 1 {
		return fmt.Errorf("unknown manifest version: %d", m.ManifestVersion)
	}

	if m.Meta.Name == "" {
		return fmt.Errorf("manifest meta.name is required")
	}

	if m.Meta.Version == "" {
		return fmt.Errorf("manifest meta.version is required")
	}

	if m.Container.Build.NixFile == "" {
		return fmt.Errorf("manifest container.build.nixFile is required")
	}

	if m.Container.Build.NixFileSha256 == "" {
		return fmt.Errorf("manifest container.build.nixFileSha256 is required")
	}

	for _, service := range m.Container.Services {
		if service.Name == "" {
			return fmt.Errorf("service name is required")
		}

		if service.Command.Exec == "" {
			return fmt.Errorf("service %s must have a non-empty exec command", service.Name)
		}
	}

	for _, expose := range m.Container.Exposes {
		if expose.Name == "" {
			return fmt.Errorf("expose name is required")
		}

		if expose.Port <= 0 || expose.Port > 65535 {
			return fmt.Errorf("expose port must be between 1 and 65535")
		}

		if expose.Type != "http" && expose.Type != "tcp" {
			return fmt.Errorf("expose type must be one of: http, tcp")
		}
	}

	return nil
}

/* PupManifestMeta holds meta information about this pup
 * such as its name, version, any imagery that needs to be shown.
 */
type PupManifestMeta struct {
	Name string `json:"name"`
	// The version of the pup.
	// nb. This can differ from the version of the software deployed
	//     by this pup, as we may change this pup manifest to expose
	//     additional configuration options for the same software version.
	Version string `json:"version"`
	// Optional. A path to a logo for this pup.
	LogoPath string `json:"logoPath"`
	// A short description, single line.
	ShortDescription string `json:"shortDescription"`
	// Optional, longer description. Used for store listings.
	LongDescription string `json:"longDescription"`
	// A key value pair of upstream versions that this pup ships with.
	UpstreamVersions map[string]string `json:"upstreamVersions"`
}

/* PupManfiestV1Container contains information about the
 * execution environment of the pup, including both build
 * and runtime details of whatever is to be executed.
 */
type PupManifestContainer struct {
	Build PupManifestBuild `json:"build"`
	// A single nix build file can provide multiple services, which all
	// may need to be started separately. Each "service" should be provided
	// as an artifact here with the correct execution configuration.
	Services []PupManifestService      `json:"services"`
	Exposes  []PupManifestExposeConfig `json:"exposes"`
	// This pup requires internet access to function.
	RequiresInternet bool `json:"requiresInternet"`
}

/* PupManifestBuild holds information about the target nix
 * package that is to be built for this pup.
 */
type PupManifestBuild struct {
	// The location of the nix file used for building this pups environment.
	NixFile string `json:"nixFile"`
	// The SHA256 hash of the nix file.
	NixFileSha256 string `json:"nixFileSha256"`
}

type PupManifestService struct {
	Name    string             `json:"name"`
	Command PupManifestCommand `json:"command"`
}

/* Represents the command to run inside this PUP
 * Container.
 */
type PupManifestCommand struct {
	// Required. The whole executable string, including any arguments that need to be passed.
	Exec string `json:"exec"`
	// Optional. The working directory specified for a systemd service.
	CWD string `json:"cwd"`
	// Optional. Any environment variables that need to be set.
	ENV map[string]string `json:"env"`
}

/* Allow the user to expose certain ports in their container. */
type PupManifestExposeConfig struct {
	Name         string   `json:"name"`         // Freeform field used to refer to this port in the frontend.
	Type         string   `json:"type"`         // Must be one of: http, tcp
	Port         int      `json:"port"`         // The port that is being listened on inside the container.
	Interfaces   []string `json:"interfaces"`   // Designates that certain interfaces can be accessed on this port
	ListenOnHost bool     `json:"listenOnHost"` // If true, the port will be accessible on the host network, otherwise it will listen on a private internal network interface.
	WebUI        bool     `json:"webUI"`        // If true, will be proxied from an available port to the dPanel user
}

type PupManifestInterface struct {
	Name             string                       `json:"name"`             // the globally unique name for this interface
	Version          string                       `json:"version"`          // Semver ie: 0.1.1
	PermissionGroups []PupManifestPermissionGroup `json:"permissionGroups"` // The permission groups that make up this interface
}

/* PermissionGroups define how other
 * pups can request access to this pup's
 * APIs and resources, via their Dependencies
 */
type PupManifestPermissionGroup struct {
	Name        string   `json:"name"`        // ie:  admin, wallet-read-only, etc.
	Description string   `json:"description"` // What does this permission group do (shown to user)
	Severity    int      `json:"severity"`    // 1-3, 1: critical/danger, 2: makes changes, 3: read only stuff
	Routes      []string `json:"routes"`      // http routes accessible for this group
	Port        int      `json:"port"`        // port accessible for this group
}

/* Dependency specifies that this pup requires
 * another pup to be running, and what permission
 * groups from that pup need to be available.
 */
type PupManifestDependency struct {
	InterfaceName    string                      `json:"interfaceName"`    // interface that we depend on
	InterfaceVersion string                      `json:"interfaceVersion"` // semver expression
	PermissionGroups []string                    `json:"permissionGroups"` // list of permission groups from that interface we want
	DefaultSource    PupManifestDependencySource `json:"source"`           // optional, default package that provides this interface
}

/* A DependencySource specifies the location of a
 * dependency that needs to be installed. We list it in the manifest
 * so that if a user doesn't already have this source set up we
 * can still resolve this dependency tree.
 */
type PupManifestDependencySource struct {
	SourceLocation string `json:"sourceLocation"`
	PupName        string `json:"pupName"`
	PupVersion     string `json:"pupVersion"`
	PupLogoBase64  string `json:"pupLogoBase64"`
}

/* Represents fields that are user settable, which provide the values
 * for templates (Args, ENV, ConfigFiles), we only care about Name
 */
type PupManifestConfigFields struct {
	Sections []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
		// TODO: we probably need a list of valid field types
		// Fields []map[string]interface{} `json:"fields"`
		Fields []struct {
			Label    string `json:"label"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Required bool   `json:"required"`
			Options  []struct {
				Label string `json:"label"`
				Value string `json:"value"`
			} `json:"options,omitempty"`
			Min  int `json:"min,omitempty"`
			Max  int `json:"max,omitempty"`
			Step int `json:"step,omitempty"`
		} `json:"fields"`
	} `json:"sections"`
}

type PupManifestMetric struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"` // string, int, float
	HistorySize int    `json:"history"`
}
