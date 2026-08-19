package middleware

import (
	"net/http"
	"testing"
)

func TestAttachmentMiddlewareBlocksPrivateObjectDirectories(t *testing.T) {
	for _, request := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/res/uploads/attachments/2026/08/file.pdf"},
		{method: http.MethodHead, path: "/res/uploads/attachment-previews/2026/08/file.pdf"},
		{method: http.MethodGet, path: "/res/uploads/test/attachments/2026/08/file.pdf"},
		{method: http.MethodGet, path: "/res/uploads/test/attachment-previews/2026/08/file.pdf"},
		{method: http.MethodGet, path: "/res/uploads/attachments"},
	} {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			ctx, recorder := newMiddlewareTestContext(request.method, request.path)
			AttachmentMiddleware(ctx)
			if !ctx.IsAborted() {
				t.Fatal("private attachment path must be aborted")
			}
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status=%d want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestAttachmentMiddlewareAllowsOtherUploads(t *testing.T) {
	for _, path := range []string{
		"/res/uploads/images/2026/08/photo.png",
		"/res/uploads/files/myattachments/readme.txt",
		"/res/images/logo.png",
	} {
		ctx, _ := newMiddlewareTestContext(http.MethodGet, path)
		AttachmentMiddleware(ctx)
		if ctx.IsAborted() {
			t.Fatalf("non-attachment path %q must remain accessible", path)
		}
	}
}
