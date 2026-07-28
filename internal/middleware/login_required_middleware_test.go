package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/errs"

	"github.com/gin-gonic/gin"
)

func TestLoginRequiredMiddlewareBlocksAnonymousContentAPI(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	ctx, recorder := newMiddlewareTestContext(http.MethodGet, "/api/topic/topics")

	LoginRequiredMiddleware(ctx)

	if !ctx.IsAborted() {
		t.Fatal("anonymous content API request must be aborted")
	}
	var body struct {
		ErrorCode int `json:"errorCode"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ErrorCode != errs.CodeNotLogin {
		t.Fatalf("errorCode=%d want %d", body.ErrorCode, errs.CodeNotLogin)
	}
}

func TestLoginRequiredMiddlewareAllowsPublicAuthAPIs(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	for _, request := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/install/status"},
		{method: http.MethodPost, path: "/api/login/signin"},
		{method: http.MethodPost, path: "/api/login/google_login_submit"},
		{method: http.MethodGet, path: "/api/captcha/request"},
		{method: http.MethodGet, path: "/api/config/configs"},
		{method: http.MethodGet, path: "/api/user/current"},
		{method: http.MethodPost, path: "/api/user/verify_email"},
	} {
		ctx, _ := newMiddlewareTestContext(request.method, request.path)
		LoginRequiredMiddleware(ctx)
		if ctx.IsAborted() {
			t.Fatalf("public API %s %s must remain accessible", request.method, request.path)
		}
	}
}

func TestLoginRequiredMiddlewareRejectsPublicPathLookalikesAndWrongMethods(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	for _, request := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/login/signin"},
		{method: http.MethodGet, path: "/api/login/signin/extra"},
		{method: http.MethodGet, path: "/api/config/configs/extra"},
		{method: http.MethodPost, path: "/api/user/current"},
	} {
		ctx, _ := newMiddlewareTestContext(request.method, request.path)
		LoginRequiredMiddleware(ctx)
		if !ctx.IsAborted() {
			t.Fatalf("request %s %s must remain protected", request.method, request.path)
		}
	}
}

func TestLoginRequiredMiddlewareAllowsAuthenticatedContentAPI(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	ctx, _ := newMiddlewareTestContext(http.MethodGet, "/api/topic/topics")
	common.SetCurrentUser(ctx, &models.User{})

	LoginRequiredMiddleware(ctx)

	if ctx.IsAborted() {
		t.Fatal("authenticated content API request must remain accessible")
	}
}

func TestLoginRequiredMiddlewareCanBeDisabled(t *testing.T) {
	withLoginRequiredConfig(t, true, false)
	ctx, _ := newMiddlewareTestContext(http.MethodGet, "/api/topic/topics")

	LoginRequiredMiddleware(ctx)

	if ctx.IsAborted() {
		t.Fatal("disabled login requirement must preserve public access")
	}
}

func TestLoginRequiredMiddlewareDoesNotBlockInstallation(t *testing.T) {
	withLoginRequiredConfig(t, false, true)
	ctx, _ := newMiddlewareTestContext(http.MethodGet, "/api/topic/topics")

	LoginRequiredMiddleware(ctx)

	if ctx.IsAborted() {
		t.Fatal("login requirement must not run before installation")
	}
}

func TestLoginRequiredResourceMiddlewareProtectsUploadsAndSitemap(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	for _, request := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/res/uploads"},
		{method: http.MethodGet, path: "/res/uploads/topic/image.png"},
		{method: http.MethodHead, path: "/res/uploads/topic/image.png"},
		{method: http.MethodGet, path: "/sitemap.xml"},
		{method: http.MethodHead, path: "/sitemap.xml"},
	} {
		ctx, recorder := newMiddlewareTestContext(request.method, request.path)
		LoginRequiredResourceMiddleware(ctx)
		if !ctx.IsAborted() {
			t.Fatalf("anonymous resource %s %s must be aborted", request.method, request.path)
		}
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("resource %s status=%d want %d", request.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestLoginRequiredResourceMiddlewareAllowsPublicAssets(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	for _, path := range []string{"/res/images/logo.png", "/assets/app.js", "/robots.txt"} {
		ctx, _ := newMiddlewareTestContext(http.MethodGet, path)
		LoginRequiredResourceMiddleware(ctx)
		if ctx.IsAborted() {
			t.Fatalf("public application asset %s must remain accessible", path)
		}
	}
}

func TestLoginRequiredResourceMiddlewareAllowsAuthenticatedUploads(t *testing.T) {
	withLoginRequiredConfig(t, true, true)
	ctx, _ := newMiddlewareTestContext(http.MethodGet, "/res/uploads/topic/image.png")
	common.SetCurrentUser(ctx, &models.User{})

	LoginRequiredResourceMiddleware(ctx)

	if ctx.IsAborted() {
		t.Fatal("authenticated upload request must remain accessible")
	}
}

func newMiddlewareTestContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, nil)
	return ctx, recorder
}

func withLoginRequiredConfig(t *testing.T, installed, loginRequired bool) {
	t.Helper()
	previous := config.Instance
	config.Instance = &config.Config{
		Installed:     installed,
		LoginRequired: loginRequired,
	}
	t.Cleanup(func() {
		config.Instance = previous
	})
}
