package html

import "testing"

func TestGetParagraphSummaryPreservesBlockBoundaries(t *testing.T) {
	htmlStr := "<p>alpha <strong>beta</strong></p><p>gamma</p><ul><li>delta</li><li>epsilon</li></ul>"
	want := "alpha beta\ngamma\ndelta\nepsilon"

	if got := GetParagraphSummary(htmlStr, 100, 800); got != want {
		t.Fatalf("GetParagraphSummary() = %q, want %q", got, want)
	}
}

func TestGetParagraphSummaryTreatsBreaksAsParagraphs(t *testing.T) {
	htmlStr := "<div>first<br>second<hr>third</div>"
	want := "first\nsecond\nthird"

	if got := GetParagraphSummary(htmlStr, 100, 800); got != want {
		t.Fatalf("GetParagraphSummary() = %q, want %q", got, want)
	}
}
