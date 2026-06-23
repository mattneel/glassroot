package report

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func visibleDisplayBytes(input []byte, limits RenderLimits) (string, error) {
	if limits.MaxDisplayInputBytes <= 0 || limits.MaxDisplayInputBytes > MaxDisplayInputBytesAbsolute || limits.MaxEscapedDisplayBytes <= 0 || limits.MaxEscapedDisplayBytes > MaxEscapedDisplayBytesAbsolute {
		return "", errCode(CodeInvalidLimits, "display", "limits", "invalid display limits", nil)
	}
	if int64(len(input)) > limits.MaxDisplayInputBytes {
		return "", errCode(CodeDisplayValueTooLarge, "display", "input", "display value exceeds input limit", nil)
	}
	var b strings.Builder
	for len(input) > 0 {
		r, size := utf8.DecodeRune(input)
		if r == utf8.RuneError && size == 1 {
			writeByteEscape(&b, input[0])
			input = input[1:]
		} else {
			writeRuneVisible(&b, r)
			input = input[size:]
		}
		if int64(b.Len()) > limits.MaxEscapedDisplayBytes {
			return "", errCode(CodeDisplayValueTooLarge, "display", "output", "display value exceeds escaped output limit", nil)
		}
	}
	return b.String(), nil
}

func writeByteEscape(b *strings.Builder, c byte) { _, _ = fmt.Fprintf(b, "\\x%02X", c) }

func writeRuneVisible(b *strings.Builder, r rune) {
	switch r {
	case '\\':
		b.WriteString(`\\`)
	case 0:
		b.WriteString(`\x00`)
	case '\a':
		b.WriteString(`\x07`)
	case '\b':
		b.WriteString(`\x08`)
	case '\t':
		b.WriteString(`\x09`)
	case '\n':
		b.WriteString(`\x0A`)
	case '\v':
		b.WriteString(`\x0B`)
	case '\f':
		b.WriteString(`\x0C`)
	case '\r':
		b.WriteString(`\x0D`)
	case 0x1b:
		b.WriteString(`\x1B`)
	case 0x7f:
		b.WriteString(`\x7F`)
	case '<', '>', '[', ']', '(', ')', '!', '#', '|', '@':
		writeRuneEscape(b, r)
	default:
		if r < 0x20 || (r >= 0x80 && r <= 0x9f) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r) {
			writeRuneEscape(b, r)
			return
		}
		b.WriteRune(r)
	}
}

func writeRuneEscape(b *strings.Builder, r rune) { _, _ = fmt.Fprintf(b, "\\u{%04X}", r) }

func markdownCodeSpan(value string) string {
	maxRun, cur := 0, 0
	for _, r := range value {
		if r == '`' {
			cur++
			if cur > maxRun {
				maxRun = cur
			}
		} else {
			cur = 0
		}
	}
	delim := strings.Repeat("`", maxRun+1)
	return delim + " " + value + " " + delim
}
