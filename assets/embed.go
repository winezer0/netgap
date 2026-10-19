package assets

import "embed"

// Files contains the built-in IPv4 and IPv6 IP location databases.
//
//go:embed ip2region_v4.xdb ip2region_v6.xdb
var Files embed.FS
