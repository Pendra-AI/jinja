package jinja_test

import (
	"strings"
	"testing"

	"github.com/ardanlabs/jinja"
)

// String literals in a template must decode their backslash escapes the way
// Jinja2 does (Python's unicode-escape codec). The engine used to decode only
// \n \t \\ \' \" and keep every other escape as written, so a chat template's
// `replace("\x00", "")` - meant to strip NUL bytes from message text - instead
// deleted the four characters `\x00` from it and let NULs through. Every
// expected value below is what Jinja2 3.1 renders for the same source.
//
// Template sources are Go raw strings, so `\x00` in them is the four
// characters the template file contains, not a NUL.

func renderEscape(t *testing.T, src string, data map[string]any) string {
	t.Helper()
	tmpl, err := jinja.Compile(src)
	if err != nil {
		t.Fatalf("compile %s: %v", src, err)
	}
	out, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render %s: %v", src, err)
	}
	return out
}

func TestStringLiteralEscapes(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		// Already supported.
		{`{{ "a\nb\tc\\d\"e" }}`, "a\nb\tc\\d\"e"},
		{`{{ 'it\'s' }}`, "it's"},

		// Hex and Unicode escapes denote code points, written as UTF-8.
		{`{{ "\x41" }}`, "A"},
		{`{{ "\xe9" }}`, "é"},
		{`{{ "\xe9" | length }}`, "1"},
		{`{{ "\u00e9" }}`, "é"},
		{`{{ "\U0001F600" }}`, "😀"},
		{`{{ "\x00" | length }}`, "1"},
		{`{{ 'it\'s "\x41"' }}`, `it's "A"`},

		// Non-ASCII written directly is unchanged.
		{`{{ "é" }}`, "é"},

		// Octal: one to three digits.
		{`{{ "\0" | length }}`, "1"},
		{`{{ "\101" }}`, "A"},
		{`{{ "\1012" }}`, "A2"},

		// The remaining single-character escapes.
		{`{{ "\r\a\b\f\v" | list | join(",") }}`, "\r,\a,\b,\f,\v"},

		// Backslash-newline is a line continuation.
		{"{{ \"a\\\nb\" }}", "ab"},
		{"{{ \"a\\\r\nb\" }}", "ab"},
		{"{{ \"a\\\rb\" }}", "ab"},

		// An unknown escape keeps its backslash, as in Python.
		{`{{ "\q" }}`, `\q`},
		{`{{ "\q" | length }}`, "2"},
		{`{{ "\8" }}`, `\8`},

		// \N{NAME} is decoded by Jinja2 (to "•" here). This engine has no
		// Unicode name table, so it stays literal.
		{`{{ "\N{BULLET}" }}`, `\N{BULLET}`},

		// Every place a string literal can appear goes through the same
		// tokenizer: set, filter arguments, macro defaults.
		{`{% set s = "\x41" %}{{ s }}`, "A"},
		{`{{ "a-b" | replace("-", "\x2d\x2d") }}`, "a--b"},
		{`{% macro m(x="\x42") %}{{ x }}{% endmacro %}{{ m() }}`, "B"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := renderEscape(t, tt.src, nil); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStringLiteralEscapes_ReplaceNUL is the case that found the bug: a chat
// template that strips NUL bytes from message text with replace("\x00", "").
func TestStringLiteralEscapes_ReplaceNUL(t *testing.T) {
	const src = `{{ s | replace("\x00", "") }}`

	// Real NUL bytes are removed.
	if got := renderEscape(t, src, map[string]any{"s": "ab\x00cd\x00"}); got != "abcd" {
		t.Errorf("NUL bytes: got %q, want %q", got, "abcd")
	}

	// The literal text backslash-x-0-0 is ordinary content and survives.
	if got := renderEscape(t, src, map[string]any{"s": `ab\x00cd`}); got != `ab\x00cd` {
		t.Errorf("literal \\x00 text: got %q, want %q", got, `ab\x00cd`)
	}
}

// A malformed \x, \u or \U escape fails to compile, as it does in Jinja2
// ("truncated \xXX escape"). llama.cpp's template engine rejects these too (it
// decodes \x and rejects \u and \U outright). It must never panic or read
// past the closing quote.
func TestStringLiteralEscapes_Malformed(t *testing.T) {
	tests := []struct {
		src     string
		wantErr string
	}{
		{`{{ "\x4" }}`, `truncated \xXX escape`},
		{`{{ "\x4g" }}`, `truncated \xXX escape`},
		{`{{ "\x" }}`, `truncated \xXX escape`},
		{`{{ "\x4" ~ "1" }}`, `truncated \xXX escape`},
		{`{{ "\u00e" }}`, `truncated \uXXXX escape`},
		{`{{ "\U0001F60" }}`, `truncated \UXXXXXXXX escape`},
		{`{{ "\U00110000" }}`, `illegal Unicode character`},
		{`{% set s = "\x4" %}{{ s }}`, `truncated \xXX escape`},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			_, err := jinja.Compile(tt.src)
			if err == nil {
				t.Fatalf("compiled; want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}
