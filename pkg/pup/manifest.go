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

	if len(m.Container.Services) == 0 {
		return fmt.Errorf("at least one service is required")
	}

	for _, service := range m.Container.Services {
		if service.Name == "" {
			return fmt.Errorf("service name is required")
		}

		if service.Command.Exec == "" {
			return fmt.Errorf("service %s must have a non-empty exec command", service.Name)
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
	// Freeform field, but we'll handle certain cases of "admin" or "public"
	Type string `json:"type"`
	// HTTP, Raw TCP etc. Used by the frontend in addition to
	// type to understand if something can be iframed.
	TrafficType string `json:"trafficType"`
	// The port that is being listened on inside the container.
	Port int `json:"port"`
}

type PupManifestInterface struct {
	Name             string                       `json:"name"`    // the globally unique name for this interface
	Version          string                       `json:"version"` // Semver ie: 0.1.1
	PermissionGroups []PupManifestPermissionGroup `json:"permissionGroups"`
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
	Type     string `json:"type"`
	Location string `json:"location"`
}

/* Represents fields that are user settable, which provide the values
 * for templates (Args, ENV, ConfigFiles), we only care about Name
 */
type PupManifestConfigFields struct {
	Sections []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
		// TODO: we probably need a list of valid field types
		Fields []map[string]interface{} `json:"fields"`
	} `json:"sections"`
}
