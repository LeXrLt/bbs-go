package text

import (
	"strings"
	"unicode"

	"github.com/mlogclub/simple/common/strs"
)

// GetSummary 获取summary
func GetSummary(s string, length int) string {
	s = strings.TrimSpace(s)
	summary := strs.Substr(s, 0, length)
	if strs.RuneLen(s) > length {
		summary += "..."
	}
	return summary
}

// GetParagraphSummary builds a paragraph-aware summary within maxLength runes.
func GetParagraphSummary(s string, minLength, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	if minLength < 0 {
		minLength = 0
	}

	paragraphs := splitParagraphs(s)
	if len(paragraphs) == 0 {
		return ""
	}

	selected := make([]rune, 0, maxLength)
	for i, paragraph := range paragraphs {
		if i > 0 {
			selected = append(selected, '\n')
		}
		selected = append(selected, []rune(paragraph)...)

		hasMore := i < len(paragraphs)-1
		if len(selected) > maxLength || (len(selected) == maxLength && hasMore) {
			return appendEllipsis(selected, maxLength)
		}
		if len(selected) >= minLength {
			if hasMore {
				return appendEllipsis(selected, maxLength)
			}
			return string(selected)
		}
	}

	return string(selected)
}

func splitParagraphs(s string) []string {
	runes := []rune(s)
	paragraphs := make([]string, 0)
	paragraph := make([]rune, 0)

	flush := func() {
		trimmed := strings.TrimSpace(string(paragraph))
		if trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
		paragraph = paragraph[:0]
	}

	for i := 0; i < len(runes); {
		if !unicode.IsSpace(runes[i]) {
			paragraph = append(paragraph, runes[i])
			i++
			continue
		}

		start := i
		isParagraphBreak := false
		for i < len(runes) && unicode.IsSpace(runes[i]) {
			if runes[i] == '\r' || runes[i] == '\n' || runes[i] == '\t' {
				isParagraphBreak = true
			}
			i++
		}
		if i-start >= 2 {
			isParagraphBreak = true
		}

		if isParagraphBreak {
			flush()
		} else {
			paragraph = append(paragraph, runes[start])
		}
	}
	flush()

	return paragraphs
}

func appendEllipsis(content []rune, maxLength int) string {
	const ellipsis = "..."
	if maxLength <= 0 {
		return ""
	}
	if len(content) <= maxLength {
		if len(content)+len(ellipsis) <= maxLength {
			return string(content) + ellipsis
		}
		return string(content)
	}
	if maxLength <= len(ellipsis) {
		return ellipsis[:maxLength]
	}

	contentLength := maxLength - len(ellipsis)
	return string(content[:contentLength]) + ellipsis
}
