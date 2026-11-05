// Package explorer carries the system map page into the evie binary: the
// page the vision skill builds from .vision/map.json, exactly as it was when
// the binary was built (system-page.spec.md). It lives beside the page
// because go:embed reaches only its own directory.
package explorer

import (
	"embed"
	"io/fs"
)

// The page is self-contained: its scripts, styles and data are inline, so
// it is the snapshot's only file. The facts, README.md and this file stay
// out.
//
//go:embed system.html
var files embed.FS

// Files is the snapshot, rooted at the explorer directory.
func Files() fs.FS {
	return files
}
