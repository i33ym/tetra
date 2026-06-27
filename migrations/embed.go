// Package migrations embeds the numbered SQL migration files so they ship inside
// the binary and can be run by the admin tool or at startup.
package migrations

import "embed"

// FS holds the embedded migration files.
//
//go:embed *.sql
var FS embed.FS
