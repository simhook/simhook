// Package migrations embeds the SQL migrations so the binary can apply them
// without any files on disk.
package migrations

import "embed"

// FS holds every *.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
