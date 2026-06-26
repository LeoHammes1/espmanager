package firmware

import (
	"embed"
	"io/fs"
)

//go:embed all:prebuilt
var prebuiltFS embed.FS

// Agent returns the embedded base ("agent") firmware filesystem: the esp-web-tools
// manifest plus the per-chip-family binaries the browser flashes during onboarding.
func Agent() (fs.FS, error) {
	return fs.Sub(prebuiltFS, "prebuilt")
}
