package assets

import "embed"

// FS contains the built Usage Keeper dashboard assets.
//
//go:embed dist/*
var FS embed.FS
