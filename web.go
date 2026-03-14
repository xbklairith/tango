package ari

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var webFS embed.FS

// WebDist returns the web/dist filesystem for SPA serving.
func WebDist() fs.FS {
	dist, _ := fs.Sub(webFS, "web/dist")
	return dist
}
