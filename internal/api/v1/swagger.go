package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	apidocs "nexus-pro-be/docs"
)

// SwaggerCtrl serves the embedded OpenAPI contract and Swagger UI.
type SwaggerCtrl struct{}

// RegisterRoutes attaches OpenAPI and Swagger UI routes.
func (SwaggerCtrl) RegisterRoutes(router *gin.Engine) {
	router.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", apidocs.OpenAPIYAML)
	})
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	router.GET("/swagger/*any", ginSwagger.WrapHandler(
		swaggerFiles.NewHandler(),
		ginSwagger.URL("/openapi.yaml"),
		ginSwagger.DocExpansion("list"),
		ginSwagger.DeepLinking(true),
	))
}
