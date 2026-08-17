package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
)

func TestBuildSimpleTopicSummaryUsesParagraphPreviewForRegularTopics(t *testing.T) {
	first := strings.Repeat("首", 199)
	topic := &models.Topic{
		Type:        constants.TopicTypeTopic,
		ContentType: constants.ContentTypeMarkdown,
		Content:     first + "\n\n第二段完整内容\n\n不应展示的第三段",
	}

	want := first + "\n第二段完整内容..."
	if got := buildSimpleTopicSummary(topic); got != want {
		t.Fatalf("buildSimpleTopicSummary() = %q, want %q", got, want)
	}
}

func TestBuildSimpleTopicSummaryCapsRegularTopicPreviewAt800Runes(t *testing.T) {
	topic := &models.Topic{
		Type:        constants.TopicTypeTopic,
		ContentType: constants.ContentTypeHtml,
		Content:     "<p>" + strings.Repeat("界", 900) + "</p>",
	}

	got := buildSimpleTopicSummary(topic)
	if utf8.RuneCountInString(got) != topicPreviewMaxLength {
		t.Fatalf("summary length = %d, want %d", utf8.RuneCountInString(got), topicPreviewMaxLength)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated summary should end with ellipsis: %q", got[len(got)-3:])
	}
}

func TestBuildSimpleTopicSummaryKeepsQuestionPreviewRule(t *testing.T) {
	topic := &models.Topic{
		Type:        constants.TopicTypeQA,
		ContentType: constants.ContentTypeMarkdown,
		Content:     strings.Repeat("问", topicSummaryLength+1),
	}

	want := strings.Repeat("问", topicSummaryLength) + "..."
	if got := buildSimpleTopicSummary(topic); got != want {
		t.Fatalf("buildSimpleTopicSummary() = %q, want legacy question summary", got)
	}
}
