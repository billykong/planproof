// Package yamljson converts the YAML a person or agent writes into the JSON
// the kernel checks (T-03). The converted bytes are what gets hashed and
// approved, so the conversion is strict and deterministic: YAML features
// that could make a file mean something other than what its reader saw are
// errors, not guesses.
//
// Allowed: one document of mappings, sequences and plain values (strings,
// integers, booleans, null). Rejected: anchors, aliases and merge keys
// (a value pulled in from elsewhere), duplicate keys (the last one would
// silently win), floats, timestamps and any other tag, non-string keys,
// more than one document, and nesting deeper than the kernel allows.
package yamljson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

const (
	MaxBytes = 64 << 10 // the kernel's limit for plans and policies
	maxDepth = 32       // the kernel's nesting limit
)

// Convert returns compact JSON with sorted keys.
func Convert(data []byte) ([]byte, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("larger than %d bytes", MaxBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("empty document")
		}
		return nil, err
	}
	var more yaml.Node
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one document")
	}
	v, err := value(doc.Content[0], 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func value(n *yaml.Node, depth int) (any, error) {
	fail := func(format string, args ...any) (any, error) {
		return nil, fmt.Errorf("line %d: %s", n.Line, fmt.Sprintf(format, args...))
	}
	if depth >= maxDepth {
		return fail("nested more than %d levels", maxDepth)
	}
	if n.Anchor != "" {
		return fail("anchors are not allowed (&%s)", n.Anchor)
	}
	switch n.Kind {
	case yaml.AliasNode:
		return fail("aliases are not allowed (*%s)", n.Value)
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return fail("keys must be plain strings (a merge key << or a %s key is not allowed)", k.Tag)
			}
			if _, dup := m[k.Value]; dup {
				return fail("duplicate key %q", k.Value)
			}
			v, err := value(n.Content[i+1], depth+1)
			if err != nil {
				return nil, err
			}
			m[k.Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		list := []any{}
		for _, c := range n.Content {
			v, err := value(c, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!str":
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!int":
			var i int64
			if err := n.Decode(&i); err != nil {
				return fail("integer out of range: %s", n.Value)
			}
			return i, nil
		case "!!bool":
			var b bool
			if err := n.Decode(&b); err != nil {
				return nil, err
			}
			return b, nil
		}
		return fail("%s values are not allowed (%q); quote it if it is text", n.Tag, n.Value)
	}
	return fail("unexpected YAML node")
}
