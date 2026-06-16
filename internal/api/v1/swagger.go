package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apidocs "nexus-pro-be/docs"
)

const swaggerIndexHTML = `<!doctype html>
<html lang="zh-Hant">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Nexus Pro Backend API 文件</title>
  <style>
    :root { color-scheme: light; font-family: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f7f8fa; color: #111827; }
    header { padding: 24px 32px 16px; border-bottom: 1px solid #d8dee8; background: #fff; }
    h1 { margin: 0 0 8px; font-size: 24px; font-weight: 700; }
    p { margin: 0; color: #4b5563; }
    main { padding: 24px 32px; }
    a { color: #1d4ed8; }
    pre { min-height: 480px; overflow: auto; margin: 16px 0 0; padding: 20px; border: 1px solid #d8dee8; border-radius: 8px; background: #fff; line-height: 1.45; }
  </style>
</head>
<body>
  <header>
    <h1>Nexus Pro Backend API 文件</h1>
    <p>OpenAPI YAML 由服務內嵌提供，可直接下載 <a href="/openapi.yaml">openapi.yaml</a>。</p>
  </header>
  <main>
    <pre id="openapi">載入中...</pre>
  </main>
  <script>
    fetch("/openapi.yaml")
      .then(function (response) { return response.text(); })
      .then(function (text) { document.getElementById("openapi").textContent = text; })
      .catch(function () { document.getElementById("openapi").textContent = "無法載入 OpenAPI 文件"; });
  </script>
</body>
</html>`

type SwaggerCtrl struct{}

func (SwaggerCtrl) RegisterRoutes(router *gin.Engine) {
	router.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", apidocs.OpenAPIYAML)
	})
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	router.GET("/swagger/*any", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerIndexHTML))
	})
}
