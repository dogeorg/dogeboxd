package dogeboxd

/* Actions are passed to the dogeboxd service via its
 * AddAction method, and represent tasks that need to
 * be done such as installing a package, starting or
 * stopping a service etc.
 */
type Action any

// Instruct dogeboxd to load/reload a local (dev) PUP
// presumably because there have been changes to the
// manifest.
type LoadLocalPup struct {
	Path string
}

type UpdatePupConfig struct {
	PupID   string
	Payload map[string]string
}
