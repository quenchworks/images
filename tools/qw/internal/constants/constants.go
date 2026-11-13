// Package constants holds the values more than one qw command depends on.
package constants

import "time"

const (
	// Org is the GitHub organization that owns the image packages.
	Org = "quenchworks"
	// Registry is where every catalog image is published: Registry + "/" + app.
	Registry = "ghcr.io/" + Org + "/images"
	// WolfiIndex is the APKINDEX URL; %s is the arch (x86_64, aarch64).
	WolfiIndex = "https://packages.wolfi.dev/os/%s/APKINDEX.tar.gz"
	// HTTPTimeout bounds one upstream request.
	HTTPTimeout = 30 * time.Second
	// CheckTimeout bounds one app's whole version check.
	CheckTimeout = 180 * time.Second
)
