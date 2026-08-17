package text

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGetParagraphSummarySeparators(t *testing.T) {
	input := "  alpha beta  gamma\tdelta\repsilon\nζήτα\u3000\u3000eta  "
	want := "alpha beta\ngamma\ndelta\nepsilon\nζήτα\neta"

	if got := GetParagraphSummary(input, 100, 200); got != want {
		t.Fatalf("GetParagraphSummary() = %q, want %q", got, want)
	}
}

func TestGetParagraphSummaryMinimumLength(t *testing.T) {
	t.Run("continues with complete paragraph below minimum", func(t *testing.T) {
		input := strings.Repeat("界", 199) + "\n下一段"
		want := strings.Repeat("界", 199) + "\n下一段"

		if got := GetParagraphSummary(input, 200, 800); got != want {
			t.Fatalf("GetParagraphSummary() = %q, want %q", got, want)
		}
	})

	t.Run("stops when first paragraph is exactly minimum", func(t *testing.T) {
		input := strings.Repeat("a", 200) + "\nsecond"
		want := strings.Repeat("a", 200) + "..."

		if got := GetParagraphSummary(input, 200, 800); got != want {
			t.Fatalf("GetParagraphSummary() = %q, want %q", got, want)
		}
	})

	t.Run("marks further omitted paragraphs", func(t *testing.T) {
		input := "short\nsecond complete paragraph\nthird"
		want := "short\nsecond complete paragraph..."

		if got := GetParagraphSummary(input, 10, 800); got != want {
			t.Fatalf("GetParagraphSummary() = %q, want %q", got, want)
		}
	})
}

func TestGetParagraphSummaryMaximumLength(t *testing.T) {
	t.Run("truncates Unicode by rune and includes ellipsis", func(t *testing.T) {
		got := GetParagraphSummary("你好世界再见", 200, 5)
		if got != "你好..." {
			t.Fatalf("GetParagraphSummary() = %q, want %q", got, "你好...")
		}
		if utf8.RuneCountInString(got) != 5 {
			t.Fatalf("summary length = %d, want 5", utf8.RuneCountInString(got))
		}
	})

	t.Run("keeps exact maximum without omitted content", func(t *testing.T) {
		want := strings.Repeat("x", 800)
		got := GetParagraphSummary(want, 200, 800)

		if got != want {
			t.Fatalf("GetParagraphSummary() unexpectedly changed exact-length content")
		}
	})

	t.Run("reserves ellipsis when content exceeds hard maximum", func(t *testing.T) {
		input := strings.Repeat("x", 801)
		want := strings.Repeat("x", 797) + "..."
		got := GetParagraphSummary(input, 200, 800)

		if got != want {
			t.Fatalf("GetParagraphSummary() suffix = %q, want a 797-rune prefix plus ellipsis", got[len(got)-3:])
		}
		if utf8.RuneCountInString(got) != 800 {
			t.Fatalf("summary length = %d, want 800", utf8.RuneCountInString(got))
		}
	})

	t.Run("keeps an exact-maximum paragraph complete", func(t *testing.T) {
		first := strings.Repeat("x", 800)
		input := first + "\nmore"

		if got := GetParagraphSummary(input, 200, 800); got != first {
			t.Fatalf("GetParagraphSummary() should not remove content to make room for an ellipsis")
		}
	})

	t.Run("keeps selected content when ellipsis exactly fits", func(t *testing.T) {
		input := strings.Repeat("x", 797) + "\nmore"
		want := strings.Repeat("x", 797) + "..."

		if got := GetParagraphSummary(input, 200, 800); got != want {
			t.Fatalf("GetParagraphSummary() did not preserve the 797-rune paragraph")
		}
	})

	t.Run("does not remove a 798-rune paragraph to fit ellipsis", func(t *testing.T) {
		want := strings.Repeat("x", 798)
		input := want + "\nmore"

		if got := GetParagraphSummary(input, 200, 800); got != want {
			t.Fatalf("GetParagraphSummary() changed a complete 798-rune paragraph")
		}
	})

	t.Run("does not remove complete accumulated paragraphs to fit ellipsis", func(t *testing.T) {
		first := strings.Repeat("a", 100)
		second := strings.Repeat("b", 698)
		want := first + "\n" + second
		input := want + "\nthird"

		if got := GetParagraphSummary(input, 200, 800); got != want {
			t.Fatalf("GetParagraphSummary() changed complete accumulated paragraphs")
		}
	})

	t.Run("truncates paragraph added below minimum", func(t *testing.T) {
		input := strings.Repeat("a", 199) + "\n" + strings.Repeat("b", 700)
		want := strings.Repeat("a", 199) + "\n" + strings.Repeat("b", 597) + "..."
		got := GetParagraphSummary(input, 200, 800)

		if got != want {
			t.Fatalf("GetParagraphSummary() did not truncate the added paragraph at 800 runes")
		}
	})
}

func TestGetParagraphSummaryEmptyAndSmallLimits(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		minLength int
		maxLength int
		want      string
	}{
		{name: "empty", input: "", minLength: 200, maxLength: 800, want: ""},
		{name: "only separators", input: " \t\r\n  ", minLength: 200, maxLength: 800, want: ""},
		{name: "zero maximum", input: "content", minLength: 200, maxLength: 0, want: ""},
		{name: "ellipsis fills tiny maximum", input: "content", minLength: 200, maxLength: 2, want: ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetParagraphSummary(tt.input, tt.minLength, tt.maxLength); got != tt.want {
				t.Fatalf("GetParagraphSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}
