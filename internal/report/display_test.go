package report

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVisibleDisplayTextEscapesControlsMarkdownHTMLAndBidi(t *testing.T) {
	limits := DefaultRenderLimits()
	input := []byte("# heading\n[text](javascript:alert(1)) <script>\\\x1b[31m\a\b\r\t")
	input = append(input, []byte("\u202e\ufeff")...)
	input = append(input, 0xff)
	got, err := visibleDisplayBytes(input, limits)
	if err != nil {
		t.Fatalf("visibleDisplayBytes() error = %v", err)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("escaped output is not valid UTF-8: %q", got)
	}
	for _, raw := range []string{"\x1b", "\a", "\b", "\r", "\t", "\n", "\u202e", "\ufeff", "<script>", "[text](javascript:alert(1))", "# heading"} {
		if strings.Contains(got, raw) {
			t.Fatalf("escaped output retained raw hostile fragment %q in %q", raw, got)
		}
	}
	for _, visible := range []string{`\\`, `\x1B`, `\x07`, `\x08`, `\x0D`, `\x09`, `\x0A`, `\u{202E}`, `\u{FEFF}`, `\xFF`, `\u{003C}`, `\u{003E}`, `\u{005B}`} {
		if !strings.Contains(got, visible) {
			t.Fatalf("escaped output missing visible marker %q in %q", visible, got)
		}
	}
}

func TestMarkdownCodeSpanCannotBeEscapedByBackticksOrSpaces(t *testing.T) {
	limits := DefaultRenderLimits()
	value, err := visibleDisplayBytes([]byte("  `value` ``` fence ```  "), limits)
	if err != nil {
		t.Fatalf("visibleDisplayBytes() error = %v", err)
	}
	span := markdownCodeSpan(value)
	if !strings.HasPrefix(span, "````") || !strings.HasSuffix(span, "````") {
		t.Fatalf("code span did not choose a delimiter longer than hostile backticks: %q", span)
	}
	if strings.Contains(span, "\n") && strings.Contains(span, "\r") {
		t.Fatalf("code span contains raw line controls: %q", span)
	}
}
