// Package migrations embeds the SQL schema and seed files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
