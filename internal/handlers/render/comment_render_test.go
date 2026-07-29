package render

import (
	"strings"
	"testing"

	"bbs-go/internal/models"
	"bbs-go/internal/models/constants"
)

func TestBuildCommentRendersAndSanitizesMarkdown(t *testing.T) {
	comment := &models.Comment{
		ContentType: constants.ContentTypeMarkdown,
		Content: `## Heading

**bold**

[remote image](https://files.example.test/docs)
![remote image](https://images.example.test/comment.png)

[unsafe](javascript:alert(1))

<script>alert("script")</script>
<img src="x" onerror="alert(1)">
<iframe src="https://example.com" onload="alert(1)"></iframe>`,
		Status: constants.StatusOk,
	}

	response := BuildComment(comment)
	if response == nil {
		t.Fatal("expected comment response")
	}
	if response.ContentType != constants.ContentTypeMarkdown {
		t.Fatalf("expected markdown content type, got %q", response.ContentType)
	}
	for _, expected := range []string{
		"<h2>Heading</h2>",
		"<strong>bold</strong>",
		`href="https://files.example.test/docs"`,
		`src="https://images.example.test/comment.png"`,
	} {
		if !strings.Contains(response.Content, expected) {
			t.Fatalf("rendered content does not contain %q: %s", expected, response.Content)
		}
	}
	for _, forbidden := range []string{"<script", "javascript:", "onerror=", "<iframe"} {
		if strings.Contains(strings.ToLower(response.Content), forbidden) {
			t.Fatalf("rendered content must not contain %q: %s", forbidden, response.Content)
		}
	}
}

func TestBuildCommentKeepsTextContentAsEscapedText(t *testing.T) {
	comment := &models.Comment{
		ContentType: constants.ContentTypeText,
		Content:     `**bold** <script>alert("xss")</script>`,
		Status:      constants.StatusOk,
	}

	response := BuildComment(comment)
	if response == nil {
		t.Fatal("expected comment response")
	}
	if response.Content != `**bold** &lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;` {
		t.Fatalf("unexpected escaped text content: %s", response.Content)
	}
	if strings.Contains(response.Content, "<strong>") || strings.Contains(response.Content, "<script>") {
		t.Fatalf("text content must not be rendered as HTML: %s", response.Content)
	}
}
