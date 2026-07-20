package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	apidocs "nexus-pro-api/docs"
)

// SwaggerCtrl 定義 swagger ctrl 的資料結構。
type SwaggerCtrl struct{}

// RegisterRoutes 註冊此 controller 的 HTTP 路由。
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
