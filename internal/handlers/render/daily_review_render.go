package render

import "bbs-go/internal/pkg/markdown"

// BuildSafeMarkdownContent renders non-topic Markdown without permitting iframe content.
func BuildSafeMarkdownContent(content string) string {
	return handleHtmlContent(markdown.ToHTML(content))
}
