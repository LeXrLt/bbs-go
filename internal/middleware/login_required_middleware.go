package middleware

import (
	"net/http"
	"strings"

	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/pkg/errs"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/services"

	"github.com/gin-gonic/gin"
)

var publicLoginRequiredAPIRequests = map[string]struct{}{
	"GET /api/install/status":                   {},
	"POST /api/install/test_db_connection":      {},
	"POST /api/install/install":                 {},
	"POST /api/login/signin":                    {},
	"POST /api/login/send_reset_password_email": {},
	"POST /api/login/reset_password":            {},
	"GET /api/login/signout":                    {},
	"POST /api/login/login_sms_code":            {},
	"POST /api/login/login_sms":                 {},
	"GET /api/login/wx_login_config":            {},
	"POST /api/login/wx_login_submit":           {},
	"GET /api/login/google_login_config":        {},
	"POST /api/login/google_login_submit":       {},
	"POST /api/login/google_one_tap":            {},
	"GET /api/login/github_login_config":        {},
	"POST /api/login/github_login_submit":       {},
	"GET /api/captcha/request":                  {},
	"GET /api/captcha/verify":                   {},
	"GET /api/captcha/request_angle":            {},
	"POST /api/user/verify_email":               {},
	"GET /api/config/configs":                   {},
	"GET /api/user/current":                     {},
}

func LoginRequiredMiddleware(ctx *gin.Context) {
	if !config.IsLoginRequired() || common.IsLogin(ctx) || isPublicLoginRequiredAPIRequest(ctx.Request) {
		ctx.Next()
		return
	}

	ginx.WriteJSON(ctx, errs.NotLogin())
	ctx.Abort()
}

func LoginRequiredResourceMiddleware(ctx *gin.Context) {
	if !config.IsLoginRequired() || !isProtectedSiteResourcePath(ctx.Request.URL.Path) {
		ctx.Next()
		return
	}

	if common.IsLogin(ctx) || services.UserTokenService.GetCurrent(ctx) != nil {
		ctx.Next()
		return
	}

	ginx.WriteHttpStatusJSON(ctx, http.StatusUnauthorized, errs.NotLogin())
	ctx.Abort()
}

func isPublicLoginRequiredAPIRequest(request *http.Request) bool {
	if request.Method == http.MethodOptions {
		return true
	}

	_, ok := publicLoginRequiredAPIRequests[request.Method+" "+request.URL.Path]
	return ok
}

func isProtectedSiteResourcePath(path string) bool {
	return path == "/sitemap.xml" || path == "/res/uploads" || strings.HasPrefix(path, "/res/uploads/")
}
