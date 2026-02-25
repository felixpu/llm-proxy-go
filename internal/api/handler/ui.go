package handler

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-proxy-go/frontend"
)

// ServeFrontend returns a handler that serves the Vue SPA with fallback to index.html.
func ServeFrontend() gin.HandlerFunc {
	frontendFS, _ := fs.Sub(frontend.FS, ".")
	fileServer := http.FileServer(http.FS(frontendFS))

	return func(c *gin.Context) {
		reqPath := strings.TrimPrefix(c.Request.URL.Path, "/")
		if reqPath == "" {
			reqPath = "index.html"
		}

		// Try to open the file; if not found, serve index.html (SPA fallback).
		if _, err := fs.Stat(frontendFS, reqPath); err != nil {
			serveIndex(c, frontendFS)
			return
		}

		// Cache strategy based on file extension.
		ext := path.Ext(reqPath)
		switch ext {
		case ".html":
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		case ".js", ".css":
			c.Header("Cache-Control", "public, max-age=86400")
		default:
			c.Header("Cache-Control", "public, max-age=604800")
		}

		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

// serveIndex serves the SPA index.html with no-cache headers.
func serveIndex(c *gin.Context, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// ServeOpenAPISpec serves the OpenAPI specification file from frontend assets.
func ServeOpenAPISpec(c *gin.Context) {
	data, err := frontend.FS.ReadFile("openapi.yaml")
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, "text/yaml; charset=utf-8", data)
}
