package db

import "embed"

// Migrations contains the immutable SQL migration set shipped with each binary.
//
//go:embed migrations/*.sql
var Migrations embed.FS
