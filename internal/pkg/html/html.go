package html

import (
	"log/slog"
	"strings"

	"bbs-go/internal/pkg/text"

	"github.com/PuerkitoBio/goquery"
	"github.com/mlogclub/simple/common/strs"
)

func GetSummary(htmlStr string, summaryLen int) string {
	if summaryLen <= 0 || strs.IsEmpty(htmlStr) {
		return ""
	}
	return text.GetSummary(GetHtmlText(htmlStr), summaryLen)
}

// GetParagraphSummary extracts block-aware text and builds a paragraph-aware summary.
func GetParagraphSummary(htmlStr string, minLength, maxLength int) string {
	if maxLength <= 0 || minLength > maxLength || strs.IsEmpty(htmlStr) {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		slog.Error(err.Error(), slog.Any("err", err))
		return ""
	}

	doc.Find("br, hr").Each(func(_ int, selection *goquery.Selection) {
		selection.ReplaceWithHtml("\n")
	})
	doc.Find("address, article, aside, blockquote, dd, div, dl, dt, fieldset, figcaption, figure, footer, form, h1, h2, h3, h4, h5, h6, header, li, main, nav, ol, p, pre, section, table, tbody, td, tfoot, th, thead, tr, ul").Each(func(_ int, selection *goquery.Selection) {
		selection.BeforeHtml("\n")
		selection.AfterHtml("\n")
	})

	return text.GetParagraphSummary(doc.Text(), minLength, maxLength)
}

// GetHtmlText 获取html文本
func GetHtmlText(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		slog.Error(err.Error(), slog.Any("err", err))
		return ""
	}
	return doc.Text()
}
