package migrations

import "embed"

//go:embed metadata/postgres/*.sql
var MetadataPostgresFS embed.FS

//go:embed metadata/sqlite/*.sql
var MetadataSQLiteFS embed.FS

//go:embed metadata/timescale/*.sql
var MetadataTimescaleFS embed.FS

//go:embed namespace/postgres/*.sql
var NamespacePostgresFS embed.FS

//go:embed namespace/sqlite/*.sql
var NamespaceSQLiteFS embed.FS

//go:embed namespace/timescale/*.sql
var NamespaceTimescaleFS embed.FS
