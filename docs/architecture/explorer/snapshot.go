// Package explorer carries the system explorer into the evie binary: the
// page, its viewer and stylesheet, its data, and its examples, exactly as
// they were when the binary was built (system-page.spec.md). It lives
// beside the files because go:embed reaches only its own directory.
package explorer

import (
	"embed"
	"io/fs"
)

// The tests, README.md, and this file stay out of the snapshot.
//
//go:embed index.html explorer.js explorer.css data examples
var files embed.FS

// Files is the explorer snapshot, rooted at the explorer directory.
func Files() fs.FS {
	return files
}
