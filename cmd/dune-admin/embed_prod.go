//go:build embed

package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed all:dist
var embeddedDist embed.FS

func embeddedSPAFS() http.FileSystem {
	sub, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		log.Fatal("embedded dist is malformed:", err)
	}
	return http.FS(sub)
}
