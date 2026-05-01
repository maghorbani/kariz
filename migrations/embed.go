// Package migrations embeds the SQL migration files so they can be used
// by the application at runtime without requiring the files on disk.
//
// The embed directive must live in this package (alongside the .sql files)
// because Go's //go:embed does not support ".." parent-directory paths.
package migrations

import "embed"

// FS contains all *.sql migration files embedded at compile time.
//
//go:embed *.sql
var FS embed.FS
