//go:build !ui

// Package web provides access to the embedded frontend build artifacts.
// When built without the "ui" tag, Dist contains a placeholder index.html.
package web

import "embed"

//go:embed index.html
var Dist embed.FS
