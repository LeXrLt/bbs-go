package api

import (
	"bbs-go/internal/handlers/render"
	"bbs-go/internal/pkg/common"
	"bbs-go/internal/pkg/ginx"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mlogclub/simple/common/strs"
	"github.com/mlogclub/simple/web"

	"bbs-go/internal/cache"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/dto"
	"bbs-go/internal/pkg/config"
	"bbs-go/internal/services"
)

func ConfigConfigs(ctx *gin.Context) {

	cfg := config.Instance

	var b *web.RspBuilder
	if cfg.Installed {
		loginConfig := services.SysConfigService.GetLoginConfig()
		openLoginConfig := dto.OpenLoginConfig{
			PasswordLogin: loginConfig.PasswordLogin,
			WeixinLogin:   dto.EnabledConfig{Enabled: loginConfig.WeixinLogin.Enabled},
			SmsLogin:      dto.EnabledConfig{Enabled: loginConfig.SmsLogin.Enabled},
			GoogleLogin:   dto.OAuthConfig{Enabled: loginConfig.GoogleLogin.Enabled},
			GithubLogin:   dto.EnabledConfig{Enabled: loginConfig.GithubLogin.Enabled},
		}
		if loginConfig.GoogleLogin.Enabled {
			openLoginConfig.GoogleLogin.ClientId = loginConfig.GoogleLogin.ClientId
		}

		var sysConfig *dto.SysConfigOpenResponse
		if cfg.LoginRequired && common.GetCurrentUser(ctx) == nil {
			siteLogo := cache.SysConfigCache.GetStr(constants.SysConfigSiteLogo)
			if strs.IsBlank(siteLogo) || strings.HasPrefix(siteLogo, "/res/uploads/") {
				siteLogo = "/res/images/logo.png"
			}
			sysConfig = &dto.SysConfigOpenResponse{
				SiteTitle:     cache.SysConfigCache.GetStr(constants.SysConfigSiteTitle),
				SiteLogo:      siteLogo,
				LoginRequired: true,
				LoginConfig:   openLoginConfig,
			}
		} else {
			sysConfig = &dto.SysConfigOpenResponse{
				SiteTitle:                  cache.SysConfigCache.GetStr(constants.SysConfigSiteTitle),
				LoginRequired:              cfg.LoginRequired,
				SiteDescription:            cache.SysConfigCache.GetStr(constants.SysConfigSiteDescription),
				BaseURL:                    services.SysConfigService.GetBaseURL(),
				SiteKeywords:               cache.SysConfigCache.GetStrArr(constants.SysConfigSiteKeywords),
				SiteLogo:                   cache.SysConfigCache.GetStr(constants.SysConfigSiteLogo),
				SiteNavs:                   services.SysConfigService.GetSiteNavs(),
				SiteNotification:           cache.SysConfigCache.GetStr(constants.SysConfigSiteNotification),
				FooterLinks:                services.SysConfigService.GetFooterLinks(),
				RecommendTags:              cache.SysConfigCache.GetStrArr(constants.SysConfigRecommendTags),
				UrlRedirect:                services.SysConfigService.IsUrlRedirect(),
				DefaultCategoryId:          services.SysConfigService.GetDefaultCategoryId(),
				TopicListStyle:             services.SysConfigService.GetTopicListStyle(),
				ArticlePending:             services.SysConfigService.IsArticlePending(),
				TopicCaptcha:               services.SysConfigService.IsTopicCaptcha(),
				UserObserveSeconds:         cache.SysConfigCache.GetInt(constants.SysConfigUserObserveSeconds),
				TokenExpireDays:            services.SysConfigService.GetTokenExpireDays(),
				CreateTopicEmailVerified:   services.SysConfigService.IsCreateTopicEmailVerified(),
				CreateArticleEmailVerified: services.SysConfigService.IsCreateArticleEmailVerified(),
				CreateCommentEmailVerified: services.SysConfigService.IsCreateCommentEmailVerified(),
				EnableHideContent:          services.SysConfigService.IsEnableHideContent(),
				EnableQaBounty:             services.SysConfigService.IsEnableQaBounty(),
				QaBountyMin:                services.SysConfigService.GetQaBountyMin(),
				QaBountyMax:                services.SysConfigService.GetQaBountyMax(),
				QaBountyRequired:           services.SysConfigService.IsQaBountyRequired(),
				Modules:                    services.SysConfigService.GetModules(),
				EmailNoticeIntervalSeconds: services.SysConfigService.GetEmailNoticeIntervalSeconds(),
				AttachmentConfig:           services.SysConfigService.GetAttachmentConfig(),
				LoginConfig:                openLoginConfig,
				ScriptInjections:           services.SysConfigService.GetScriptInjections(),
			}
		}
		if strs.IsBlank(sysConfig.SiteLogo) {
			sysConfig.SiteLogo = "/res/images/logo.png"
		}
		b = web.NewRspBuilder(sysConfig)
	} else {
		b = web.NewEmptyRspBuilder()
	}
	b.Put("installed", cfg.Installed)
	b.Put("language", cfg.Language)
	ginx.WriteJSON(ctx, b.Build())

}

func ConfigAbout(ctx *gin.Context) {

	ginx.WriteJSON(ctx, render.BuildAboutPage(services.SysConfigService.GetAboutPageConfig()))

}
