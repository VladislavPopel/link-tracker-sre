package project

import "embed"

//go:embed migrations
var MigrationsFS embed.FS

//go:embed web
var WebFS embed.FS
