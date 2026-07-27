package render

import (
	"bbs-go/internal/models/resp"
	"bbs-go/internal/pkg/markdown"
	"reflect"
	"strings"
	"testing"
)

func TestHandleTopicHtmlContentBuildsTocAndHeadingIds(t *testing.T) {
	htmlContent := `
<h1>Page title</h1>
<h2>Intro</h2>
<p>body</p>
<h3>Intro</h3>
<h4>中文 小节</h4>
<h5>Ignored</h5>
<h2>!!!</h2>
<h2>   </h2>
`

	content, toc := handleTopicHtmlContent(htmlContent)

	expected := []resp.TopicTocItem{
		{Id: "topic-heading-intro", Title: "Intro", Level: 2},
		{Id: "topic-heading-intro-2", Title: "Intro", Level: 3},
		{Id: "topic-heading-中文-小节", Title: "中文 小节", Level: 4},
		{Id: "section", Title: "!!!", Level: 2},
	}
	if !reflect.DeepEqual(toc, expected) {
		t.Fatalf("unexpected toc: %#v", toc)
	}

	for _, item := range expected {
		if !strings.Contains(content, `id="`+item.Id+`"`) {
			t.Fatalf("content does not contain id %q: %s", item.Id, content)
		}
	}
	if strings.Contains(content, `id="Page title"`) || strings.Contains(content, `id="Ignored"`) {
		t.Fatalf("content should not assign ids to h1/h5: %s", content)
	}
}

func TestHandleTopicHtmlContentReturnsUpdatedContent(t *testing.T) {
	content, toc := handleTopicHtmlContent(`<h2>Title</h2><p>body</p>`)

	if len(toc) != 1 || toc[0].Id != "topic-heading-title" {
		t.Fatalf("unexpected toc: %#v", toc)
	}
	if !strings.Contains(content, `<h2 id="topic-heading-title">Title</h2>`) {
		t.Fatalf("heading id was not written to content: %s", content)
	}
}

func TestHandleTopicHtmlContentAllowsSandboxedHTTPSIframe(t *testing.T) {
	content, _ := handleTopicHtmlContent(`<iframe src="https://aikol.yz.rs/" title="X/Twitter" onload="alert(1)" sandbox="allow-top-navigation"></iframe>`)

	for _, expected := range []string{
		`<iframe`,
		`src="https://aikol.yz.rs/"`,
		`title="X/Twitter"`,
		`loading="lazy"`,
		`referrerpolicy="no-referrer"`,
		`allowfullscreen=""`,
		`allow-scripts`,
		`allow-same-origin`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("iframe content does not contain %q: %s", expected, content)
		}
	}
	for _, forbidden := range []string{"onload", "alert(1)", "allow-top-navigation"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("iframe content must not contain %q: %s", forbidden, content)
		}
	}
}

func TestHandleTopicHtmlContentRejectsNonHTTPSIframes(t *testing.T) {
	for _, src := range []string{
		"http://example.com",
		"//example.com",
		"javascript:alert(1)",
		"data:text/html,hello",
		"https:///missing-host",
	} {
		content, _ := handleTopicHtmlContent(`<p>before</p><iframe src="` + src + `"></iframe><p>after</p>`)
		if strings.Contains(content, "<iframe") {
			t.Fatalf("iframe with src %q must be removed: %s", src, content)
		}
		if !strings.Contains(content, "before") || !strings.Contains(content, "after") {
			t.Fatalf("removing iframe with src %q must preserve surrounding content: %s", src, content)
		}
	}
}

func TestMarkdownTopicContentAllowsHTTPSIframe(t *testing.T) {
	markdownHTML := markdown.ToHTML(`<iframe src="https://aikol.yz.rs/"></iframe>`)
	content, _ := handleTopicHtmlContent(markdownHTML)

	if !strings.Contains(content, `<iframe`) || !strings.Contains(content, `src="https://aikol.yz.rs/"`) {
		t.Fatalf("markdown iframe was not preserved: %s", content)
	}
}

func TestNonTopicContentStillRejectsIframe(t *testing.T) {
	content := handleHtmlContent(`<p>comment</p><iframe src="https://aikol.yz.rs/"></iframe>`)

	if strings.Contains(content, "<iframe") {
		t.Fatalf("non-topic content must not allow iframe: %s", content)
	}
}
