package apidocs

import _ "embed"

var (
	// OpenAPIYAML contains the embedded OpenAPI contract served by the HTTP API.
	//
	//go:embed openapi.yaml
	OpenAPIYAML []byte
)
