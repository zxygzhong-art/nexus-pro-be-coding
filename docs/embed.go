package apidocs

import _ "embed"

var (
	// OpenAPIYAML 保存 HTTP API 對外服務的內嵌 OpenAPI 契約。
	//
	//go:embed openapi.yaml
	OpenAPIYAML []byte
)
