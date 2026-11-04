package jinja_test

import (
	"testing"

	"github.com/ardanlabs/jinja"
)

// dict() must behave like Python's: an optional mapping (or iterable of
// key/value pairs) is shallow-copied first, then the keyword arguments are
// applied in the order written, overriding on a clash. DeepSeek V4.1 Flash's
// chat template copies each message with dict(msg, content_blocks=…); when the
// mapping was dropped every message lost its role and the template raised
// "Unknown role: ". Every expected string below is Jinja2 3.1.6 output with
// tojson as json.dumps, copied verbatim.

func TestDictBuiltin(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{"mapping plus kwarg", `{{ dict(m, content="x") | tojson }}`, `{"role": "assistant", "content": "x"}`},
		{"mapping only", `{{ dict(m) | tojson }}`, `{"role": "assistant", "content": "hi"}`},
		{"kwarg overrides mapping in place", `{{ dict(m, role="user") | tojson }}`, `{"role": "user", "content": "hi"}`},
		{"kwargs keep call order", `{{ dict(b=1, a=2, c=3) | tojson }}`, `{"b": 1, "a": 2, "c": 3}`},
		{"pairs then kwargs", `{{ dict([["k", 1], ["j", 2]], z=0) | tojson }}`, `{"k": 1, "j": 2, "z": 0}`},
		{"items round trip", `{{ dict(m.items()) | tojson }}`, `{"role": "assistant", "content": "hi"}`},
		{"empty", `{{ dict().items() | list | length }}`, `0`},
		{"copy leaves source alone", `{% set c = dict(m, extra=[1]) %}{{ m | tojson }}|{{ c.role }}`, `{"role": "assistant", "content": "hi"}|assistant`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderDict(t, tt.tmpl)
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

// Jinja2 raises TypeError for each of these.
func TestDictBuiltinRejects(t *testing.T) {
	for _, tmpl := range []string{`{{ dict(1) }}`, `{{ dict(m, m) }}`, `{{ dict(none) }}`, `{{ dict([1, 2]) }}`} {
		t.Run(tmpl, func(t *testing.T) {
			tp, err := jinja.Compile(tmpl)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if out, err := tp.Render(map[string]any{"m": map[string]any{"role": "a"}}); err == nil {
				t.Errorf("rendered %q, want an error", out)
			}
		})
	}
}

// The message keeps its key order through the copy, so a template that
// serialises it sees the same document it was given.
func TestDictBuiltinKeepsMappingOrder(t *testing.T) {
	tp, err := jinja.Compile(`{{ dict(m, z=1) | tojson }}`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := tp.RenderValues(map[string]jinja.Value{"m": orderedValue(t, `{"role": "tool", "tool_call_id": "c1", "content": "ok"}`)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := `{"role": "tool", "tool_call_id": "c1", "content": "ok", "z": 1}`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func renderDict(t *testing.T, tmpl string) string {
	t.Helper()
	tp, err := jinja.Compile(tmpl)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := tp.RenderValues(map[string]jinja.Value{"m": orderedValue(t, `{"role": "assistant", "content": "hi"}`)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return got
}
