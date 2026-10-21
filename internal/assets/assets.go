package assets

import "embed"

// Files contains the API explorer and its OpenAPI contract.
//
//go:embed index.html docs.html openapi.json
var Files embed.FS
