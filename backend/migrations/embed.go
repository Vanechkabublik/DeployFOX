package migrations

import "embed"

// Files holds the versioned schema used by both API and worker.
//
//go:embed *.sql
var Files embed.FS
