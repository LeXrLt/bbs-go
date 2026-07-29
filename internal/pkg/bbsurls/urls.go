package bbsurls

import (
	"bbs-go/internal/cache"
	"bbs-go/internal/models/constants"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"bbs-go/internal/pkg/idcodec"
)

// 是否是内部链接
func IsInternalUrl(href string) bool {
	internal, err := isInternalURL(href, getBaseURL())
	if err != nil {
		slog.Error(err.Error(), slog.Any("err", err))
		return false
	}
	return internal
}

func isInternalURL(href, baseURL string) (bool, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return false, nil
	}
	// Browsers treat backslashes as slashes when resolving HTTP URLs. Match that
	// behavior so \\host paths cannot be mistaken for internal relative links.
	href = strings.ReplaceAll(href, `\`, "/")

	if strings.HasPrefix(href, "//") {
		authority := strings.TrimLeft(href, "/")
		if authority == "" {
			return false, nil
		}
		href = "//" + authority
	}

	target, err := url.Parse(href)
	if err != nil {
		return false, nil
	}
	if target.Scheme == "" && target.Host == "" {
		return true, nil
	}
	if target.Host == "" || (target.Scheme != "" && !isHTTPScheme(target.Scheme)) {
		return false, nil
	}

	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false, err
	}
	if base.Host == "" {
		return false, nil
	}
	return strings.EqualFold(target.Host, base.Host), nil
}

func isHTTPScheme(scheme string) bool {
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}

// 是否是锚链接
func IsAnchor(href string) bool {
	return strings.Index(href, "#") == 0
}

func AbsUrl(path string) string {
	baseURL := getBaseURL()
	if baseURL == "/" {
		return path
	}
	return baseURL + path
}

func getBaseURL() string {
	baseURL := strings.TrimSpace(cache.SysConfigCache.GetStr(constants.SysConfigBaseURL))
	if baseURL == "" {
		return "/"
	}
	for len(baseURL) > 1 && strings.HasSuffix(baseURL, "/") {
		baseURL = strings.TrimSuffix(baseURL, "/")
	}
	return baseURL
}

// 用户主页
func UserUrl(userId int64) string {
	return AbsUrl("/user/" + idcodec.Encode(userId))
}

// 文章详情
func ArticleUrl(articleId int64) string {
	return AbsUrl("/article/" + strconv.FormatInt(articleId, 10))
}

// 标签文章列表
func TagArticlesUrl(tagId int64) string {
	return AbsUrl("/articles/" + strconv.FormatInt(tagId, 10))
}

// 话题详情
func TopicUrl(topicId int64) string {
	return AbsUrl("/topic/" + idcodec.Encode(topicId))
}

func UrlJoin(parts ...string) string {
	sep := "/"
	var ss []string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		var (
			from = 0
			to   = len(part)
		)
		if strings.Index(part, sep) == 0 {
			from = 1
		}
		if strings.LastIndex(part, sep) == len(part)-1 {
			to = len(part) - 1
		}
		part = part[from:to]

		ss = append(ss, part)
		if i != len(parts)-1 {
			ss = append(ss, sep)
		}
	}
	return strings.Join(ss, "")
}
