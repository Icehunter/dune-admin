//go:build !embed

package main

import "net/http"

func embeddedSPAFS() http.FileSystem { return nil }
