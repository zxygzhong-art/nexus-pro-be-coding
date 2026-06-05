// Package server builds the Gin HTTP server and manages its lifecycle.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// New builds the configured *http.Server (handler + addr) ready to serve.
func New(addr string, d Deps) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	register(r, d)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// Shutdown gracefully stops the server.
func Shutdown(ctx context.Context, srv *http.Server) error {
	return srv.Shutdown(ctx)
}
