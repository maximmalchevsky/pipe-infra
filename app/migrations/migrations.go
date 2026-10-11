// Package migrations embeds SQL database migration scripts.
package migrations

import "embed"

// FS contains embedded SQL migrations.
//
//go:embed *.sql
var FS embed.FS
