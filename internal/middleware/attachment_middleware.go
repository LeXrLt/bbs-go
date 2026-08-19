package middleware

import (
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// AttachmentMiddleware prevents direct access to private attachment objects.
// Original files and generated previews must only be served by authenticated
// API handlers after topic visibility and purchase checks.
func AttachmentMiddleware(ctx *gin.Context) {
	requestPath := path.Clean(ctx.Request.URL.Path)
	if requestPath != "/res/uploads" && !strings.HasPrefix(requestPath, "/res/uploads/") {
		ctx.Next()
		return
	}
	for _, segment := range strings.Split(strings.TrimPrefix(requestPath, "/res/uploads/"), "/") {
		if segment == "attachments" || segment == "attachment-previews" {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}
	}
	ctx.Next()
}
