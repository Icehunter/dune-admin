//go:build embed

package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var embeddedDist embed.FS

func embeddedSPAFS() http.FileSystem {
	sub, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		componentLog("embed").Fatal().Err(err).Msg("embedded dist is malformed")
	}
	return http.FS(sub)
}
