// Package assets embeds the compiled frontend for mw-ww.exe.
package assets

import "embed"

//go:embed all:frontend/dist
var Frontend embed.FS
