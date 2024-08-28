package pup

/* PupManifest represents a Nix installed process
 * running inside the Dogebox Runtime Environment.
 * These are defined in pup.json files.
 */
type PupManifest struct {
	// The version of the actual manifest. This differs from the "version"
	// of the pup, and the version of the deployed software for this pup.
	// Valid values: 1
	ManifestVersion  string                       `json:"manifestVersion"`
	Meta             PupManifestMeta              `json:"meta"`
	Config           PupManifestConfigFields      `json:"config"`
	Container        PupManifestContainer         `json:"container"`
	PermissionGroups []PupManifestPermissionGroup `json:"permissionGroups"`
	Dependencies     []PupManifestDependency      `json:"dependencies"`
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
}

/* PupManfiestV1Container contains information about the
 * execution environment of the pup, including both build
 * and runtime details of whatever is to be executed.
 */
type PupManifestContainer struct {
	Build   PupManifestBuild          `json:"build"`
	Command PupManifestCommand        `json:"command"`
	Exposes []PupManifestExposeConfig `json:"exposes"`
}

/* PupManifestBuild holds information about the target nix
 * package that is to be built for this pup.
 */
type PupManifestBuild struct {
	// The location of the nix file used for building this pups environment.
	NixFile string `json:"nixFile"`
	// The SHA256 hash of the nix file.
	NixFileSha256 string `json:"nixFileSha256"`
	// A single nix build file can provide multiple services, which all
	// may need to be started separately. Each "service" should be provided
	// as an artifact here with the correct execution configuration.
	Artifacts []PupManifestBuildArtifact `json:"artifacts"`
}

type PupManifestBuildArtifact struct {
	Provides string             `json:"provides"`
	Command  PupManifestCommand `json:"command"`
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
	TcpType string `json:"tcpType"`
	// The port that is being listened on inside the container.
	Port int `json:"port"`
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
	ID               string                          `json:"id"` // pup that we depend on
	Repository       PupManifestDependencyRepository `json:"repository"`
	PermissionGroups []string                        `json:"permissionGroups"` // list of permission groups from that pup we want access to
	Version          string                          `json:"version"`          // min version of the pup required
}

/* A DependencyRepository specifies the location of a
 * dependency that needs to be installed. We list it in the manifest
 * so that if a user doesn't already have this repository set up we
 * can still resolve this dependency tree.
 */
type PupManifestDependencyRepository struct {
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
