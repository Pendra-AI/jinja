package jinja_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/ardanlabs/jinja"
)

// tojson(indent=N) must produce what Python's json.dumps(indent=N) produces
// (HuggingFace's tojson is json.dumps with sort_keys=False): dict keys in
// insertion order, "," + newline between items, ": " between key and value,
// no HTML escaping. Chat templates such as Llama 3.x render their tool schemas
// this way, so a sorted or differently-spaced schema changes the prompt the
// model sees. Every expected string below is json.dumps output, copied verbatim.

// orderedValue decodes JSON into a jinja.Value whose dicts keep the document's
// key order, the way a caller hands a template client-supplied JSON through
// RenderValues. (Render's map[string]any input cannot carry an order at all.)
func orderedValue(t *testing.T, src string) jinja.Value {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(src)))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err != nil {
		t.Fatalf("decode %s: %v", src, err)
	}
	return v
}

func decodeOrdered(dec *json.Decoder) (jinja.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return jinja.Value{}, err
	}
	switch tok := tok.(type) {
	case json.Delim:
		if tok == '[' {
			var items []jinja.Value
			for dec.More() {
				item, err := decodeOrdered(dec)
				if err != nil {
					return jinja.Value{}, err
				}
				items = append(items, item)
			}
			_, err := dec.Token()
			return jinja.NewList(items), err
		}
		d := jinja.NewDict()
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return jinja.Value{}, err
			}
			val, err := decodeOrdered(dec)
			if err != nil {
				return jinja.Value{}, err
			}
			d.AsDict().Set(key.(string), val)
		}
		_, err := dec.Token()
		return d, err
	case json.Number:
		if n, err := tok.Int64(); err == nil {
			return jinja.NewInt(n), nil
		}
		f, err := tok.Float64()
		return jinja.NewFloat(f), err
	case string:
		return jinja.NewString(tok), nil
	case bool:
		return jinja.NewBool(tok), nil
	case nil:
		return jinja.None(), nil
	}
	return jinja.Value{}, io.ErrUnexpectedEOF
}

func renderTojson(t *testing.T, source string, v jinja.Value) string {
	t.Helper()
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile %s: %v", source, err)
	}
	got, err := tmpl.RenderValues(map[string]jinja.Value{"v": v})
	if err != nil {
		t.Fatalf("render %s: %v", source, err)
	}
	return got
}

// A tool definition shaped like the ones Llama 3.x templates render, with keys
// deliberately out of alphabetical order at every level.
const orderedTool = `{"type":"function","function":{"name":"book","description":"Book <a> & go",` +
	`"parameters":{"type":"object","required":["to","from"],"properties":{"to":{"type":"string"},` +
	`"from":{"type":"string"},"adults":{"type":"integer","minimum":1,"maximum":9},` +
	`"budget":{"type":"number","default":1.5},"tags":{"type":"array","items":{},"enum":[]},` +
	`"ok":{"default":true},"none":null}}}}`

func TestTojsonIndentKeepsKeyOrderLikePython(t *testing.T) {
	// json.dumps(tool, ensure_ascii=False, indent=4)
	want := `{
    "type": "function",
    "function": {
        "name": "book",
        "description": "Book <a> & go",
        "parameters": {
            "type": "object",
            "required": [
                "to",
                "from"
            ],
            "properties": {
                "to": {
                    "type": "string"
                },
                "from": {
                    "type": "string"
                },
                "adults": {
                    "type": "integer",
                    "minimum": 1,
                    "maximum": 9
                },
                "budget": {
                    "type": "number",
                    "default": 1.5
                },
                "tags": {
                    "type": "array",
                    "items": {},
                    "enum": []
                },
                "ok": {
                    "default": true
                },
                "none": null
            }
        }
    }
}`
	if got := renderTojson(t, `{{ v | tojson(indent=4) }}`, orderedValue(t, orderedTool)); got != want {
		t.Errorf("tojson(indent=4) differs from json.dumps(indent=4):\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestTojsonIndentEdgeCases(t *testing.T) {
	cases := []struct {
		name, source, in, want string
	}{
		// json.dumps({"z":1,"y":2}, indent=2)
		{"indent 2", `{{ v | tojson(indent=2) }}`, `{"z":1,"y":2}`, "{\n  \"z\": 1,\n  \"y\": 2\n}"},
		// json.dumps(..., indent=0): line breaks, no indentation.
		{"indent 0", `{{ v | tojson(indent=0) }}`, `{"b":[1,{"a":[]}],"a":{}}`, "{\n\"b\": [\n1,\n{\n\"a\": []\n}\n],\n\"a\": {}\n}"},
		{"empty list", `{{ v | tojson(indent=2) }}`, `[]`, `[]`},
		{"empty dict", `{{ v | tojson(indent=2) }}`, `{}`, `{}`},
		{"scalar", `{{ v | tojson(indent=2) }}`, `"x<y"`, `"x<y"`},
		// indent=None is the compact form.
		{"indent none", `{{ v | tojson(indent=none) }}`, `{"z":1,"y":[2]}`, `{"z": 1, "y": [2]}`},
		// The compact form keeps its ", " separator and insertion order.
		{"compact", `{{ v | tojson }}`, `{"z":1,"y":[2,{"b":null,"a":1.5}]}`, `{"z": 1, "y": [2, {"b": null, "a": 1.5}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderTojson(t, c.source, orderedValue(t, c.in)); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
