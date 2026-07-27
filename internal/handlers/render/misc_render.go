package render

import (
	"bbs-go/internal/cache"
	"bbs-go/internal/models/constants"
	"bbs-go/internal/models/resp"
	"bbs-go/internal/pkg/bbsurls"
	"bbs-go/internal/pkg/event"
	"bbs-go/internal/pkg/ginx"
	"bbs-go/internal/pkg/locales"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/mlogclub/simple/common/dates"

	"github.com/microcosm-cc/bluemonday"

	"github.com/PuerkitoBio/goquery"
	"github.com/mlogclub/simple/common/strs"
	"github.com/mlogclub/simple/common/urls"
	"github.com/mlogclub/simple/web"

	"bbs-go/internal/models"
	"bbs-go/internal/services"
)

var httpsIframeSrcPattern = regexp.MustCompile(`(?i)^https://[^\s]+$`)

func xssProtection(htmlContent string, allowHTTPSIframe bool) string {
	ugcProtection := bluemonday.UGCPolicy() // 用户生成内容模式
	ugcProtection.AllowAttrs("class").OnElements("code")
	ugcProtection.AllowAttrs("start").OnElements("ol", "ul", "li")
	if allowHTTPSIframe {
		ugcProtection.AllowAttrs("src").Matching(httpsIframeSrcPattern).OnElements("iframe")
		ugcProtection.AllowAttrs("title").OnElements("iframe")
		ugcProtection.RequireSandboxOnIFrame(
			bluemonday.SandboxAllowDownloads,
			bluemonday.SandboxAllowForms,
			bluemonday.SandboxAllowPopups,
			bluemonday.SandboxAllowPopupsToEscapeSandbox,
			bluemonday.SandboxAllowPresentation,
			bluemonday.SandboxAllowSameOrigin,
			bluemonday.SandboxAllowScripts,
		)
	}
	return ugcProtection.Sanitize(htmlContent)
}

// handleHtmlContent 处理html内容
func handleHtmlContent(htmlContent string) string {
	htmlContent, _ = handleHtmlContentWithToc(htmlContent, false, false)
	return htmlContent
}

func handleTopicHtmlContent(htmlContent string) (string, []resp.TopicTocItem) {
	return handleHtmlContentWithToc(htmlContent, true, true)
}

func handleHtmlContentWithToc(htmlContent string, buildToc, allowHTTPSIframe bool) (string, []resp.TopicTocItem) {
	htmlContent = xssProtection(htmlContent, allowHTTPSIframe)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent, nil
	}

	doc.Find("iframe").Each(func(_ int, selection *goquery.Selection) {
		src := strings.TrimSpace(selection.AttrOr("src", ""))
		iframeURL, parseErr := url.Parse(src)
		if parseErr != nil || !strings.EqualFold(iframeURL.Scheme, "https") || iframeURL.Hostname() == "" {
			selection.Remove()
			return
		}

		selection.SetAttr("src", iframeURL.String())
		selection.SetAttr("loading", "lazy")
		selection.SetAttr("referrerpolicy", "no-referrer")
		selection.SetAttr("allowfullscreen", "")
		selection.SetAttr("sandbox", strings.Join([]string{
			"allow-downloads",
			"allow-forms",
			"allow-popups",
			"allow-popups-to-escape-sandbox",
			"allow-presentation",
			"allow-same-origin",
			"allow-scripts",
		}, " "))
		if strings.TrimSpace(selection.AttrOr("title", "")) == "" {
			selection.SetAttr("title", iframeURL.Hostname())
		}
	})

	doc.Find("a").Each(func(_ int, selection *goquery.Selection) {
		href := selection.AttrOr("href", "")

		if strs.IsBlank(href) {
			return
		}

		// 不是内部链接
		if !bbsurls.IsInternalUrl(href) {
			selection.SetAttr("target", "_blank")
			selection.SetAttr("rel", "external nofollow") // 标记站外链接，搜索引擎爬虫不传递权重值

			if services.SysConfigService.IsUrlRedirect() { // 开启非内部链接跳转
				newHref := urls.ParseUrl(bbsurls.AbsUrl("/redirect")).AddQuery("url", href).BuildStr()
				selection.SetAttr("href", newHref)
			}
		}

		// 如果a标签没有title，那么设置title
		title := selection.AttrOr("title", "")
		if len(title) == 0 {
			selection.SetAttr("title", selection.Text())
		}
	})

	// 处理图片
	doc.Find("img").Each(func(_ int, selection *goquery.Selection) {
		src := selection.AttrOr("src", "")

		// 处理第三方图片
		if strings.Contains(src, "qpic.cn") {
			src = urls.ParseUrl("/api/img/proxy").AddQuery("url", src).BuildStr()
		}

		// 处理图片样式
		src = HandleOssImageStyleDetail(src)

		// // 处理lazyload
		// selection.SetAttr("data-src", src)
		// selection.RemoveAttr("src")

		selection.SetAttr("src", src)
	})

	var toc []resp.TopicTocItem
	if buildToc {
		toc = buildTopicToc(doc)
	}

	if htmlStr, err := doc.Find("body").Html(); err == nil {
		return htmlStr, toc
	}
	return htmlContent, toc
}

func buildTopicToc(doc *goquery.Document) []resp.TopicTocItem {
	var toc []resp.TopicTocItem
	usedIds := make(map[string]int)
	doc.Find("h2,h3,h4").Each(func(_ int, selection *goquery.Selection) {
		title := strings.TrimSpace(selection.Text())
		if strs.IsBlank(title) {
			return
		}

		level := headingLevel(selection)
		id := uniqueHeadingId(slugHeading(title), usedIds)
		selection.SetAttr("id", id)
		toc = append(toc, resp.TopicTocItem{
			Id:    id,
			Title: title,
			Level: level,
		})
	})
	return toc
}

func headingLevel(selection *goquery.Selection) int {
	switch goquery.NodeName(selection) {
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	default:
		return 0
	}
}

func slugHeading(title string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if strs.IsBlank(slug) {
		return "section"
	}
	return "topic-heading-" + slug
}

func uniqueHeadingId(base string, usedIds map[string]int) string {
	count := usedIds[base]
	usedIds[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

/*
BuildLoginSuccess 处理登录成功后的返回数据

Parameter:

	user - login user
	redirect - 登录来源地址，需要控制登录成功之后跳转到该地址
*/
func BuildLoginSuccess(ctx *gin.Context, user *models.User, redirect string) *web.JsonResult {
	if user == nil || user.Status != constants.StatusOk {
		return web.JsonErrorMsg(locales.Get("errors.user_not_found_or_disabled"))
	}
	token, err := services.UserTokenService.Generate(user.Id)
	if err != nil {
		return web.JsonError(err)
	}
	ginx.SetCookieKV(ctx, constants.CookieTokenKey, token, ginx.CookieHTTPOnly(true), ginx.CookieExpires(365*24*time.Hour))
	event.Send(event.UserLoginEvent{
		UserId:     user.Id,
		LoginTime:  dates.NowTimestamp(),
		IsNewLogin: true,
	})

	// 与「登录态访问」每日只发一次去重
	cache.DailyVisitCache.MarkSentToday(user.Id)

	return web.NewEmptyRspBuilder().
		Put("token", token).
		Put("user", BuildUserProfile(user)).
		Put("redirect", redirect).JsonResult()
}
