//go:build ui

// Package web embeds the frontend build artifacts into the Go binary.
package web

import "embed"

//go:embed all:ui/dist
var Dist embed.FS
