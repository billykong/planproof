package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Input size limits (K-11, spec 01 and 02).
const (
	maxPlanBytes     = 64 << 10
	maxSnapshotBytes = 4 << 20
	maxPolicyBytes   = 64 << 10
	maxDepth         = 32
)

// Reason codes for the input rule. Fixed strings: the packet renders each
// one with its own template.
const (
	reasonTooLarge         = "too-large"
	reasonInvalidUTF8      = "invalid-utf8"
	reasonInvalidJSON      = "invalid-json"
	reasonTooDeep          = "too-deep"
	reasonDuplicateKey     = "duplicate-key"
	reasonUnknownField     = "unknown-field"
	reasonWrongType        = "wrong-type"
	reasonTrailingData     = "trailing-data"
	reasonMissing          = "missing"
	reasonBadValue         = "bad-value"
	reasonDuplicate        = "duplicate"
	reasonWrongVersion     = "wrong-version"
	reasonSnapshotMismatch = "snapshot-mismatch"
)

// inputError is a failure of the input rule: which input, a reason code,
// and where in the input (a path such as steps[2].replicas).
type inputError struct {
	input, reason, path string
}

// decodeStrict parses data into v, rejecting what encoding/json lets
// through on its own: invalid UTF-8, duplicate keys, keys that differ from
// the schema only in case, deep nesting and trailing data (SEC-03, K-10).
func decodeStrict(input string, data []byte, limit int, v any) *inputError {
	if len(data) > limit {
		return &inputError{input, reasonTooLarge, ""}
	}
	if !utf8.Valid(data) {
		return &inputError{input, reasonInvalidUTF8, ""}
	}
	if reason, path := scan(data); reason != "" {
		return &inputError{input, reason, path}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var te *json.UnmarshalTypeError
		switch {
		case errors.As(err, &te):
			return &inputError{input, reasonWrongType, te.Field}
		case errors.Is(err, errBadValue):
			return &inputError{input, reasonBadValue, ""}
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			// scan already rejected keys that aren't in the schema, so the
			// name here is a schema key in the wrong place, safe to show.
			return &inputError{input, reasonUnknownField, strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)}
		}
		return &inputError{input, reasonInvalidJSON, ""}
	}
	if _, err := dec.Token(); err != io.EOF {
		return &inputError{input, reasonTrailingData, ""}
	}
	return nil
}

// scan walks the JSON tokens once, checking nesting depth, duplicate keys,
// and that every key is spelled exactly as some field in the schema.
func scan(data []byte) (reason, path string) {
	type frame struct {
		obj     bool
		keys    map[string]bool
		wantKey bool
		key     string
		idx     int
	}
	var stack []*frame
	where := func() string {
		var b strings.Builder
		for _, f := range stack {
			if f.obj {
				if !f.wantKey {
					b.WriteString("." + f.key)
				}
			} else {
				b.WriteString("[" + strconv.Itoa(f.idx) + "]")
			}
		}
		return strings.TrimPrefix(b.String(), ".")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return "", ""
		}
		if err != nil {
			return reasonInvalidJSON, where()
		}
		d, isDelim := tok.(json.Delim)
		if isDelim && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
		} else if n := len(stack); n > 0 && stack[n-1].obj && stack[n-1].wantKey {
			top, key := stack[n-1], tok.(string)
			if !schemaKeys[key] {
				return reasonUnknownField, join(where(), safeKey(key))
			}
			if top.keys[key] {
				return reasonDuplicateKey, join(where(), key)
			}
			top.keys[key], top.key, top.wantKey = true, key, false
			continue
		} else if isDelim {
			if len(stack) >= maxDepth {
				return reasonTooDeep, where()
			}
			stack = append(stack, &frame{obj: d == '{', keys: map[string]bool{}, wantKey: d == '{'})
			continue
		}
		// A value just ended: the parent expects its next key or element.
		if n := len(stack); n > 0 {
			if stack[n-1].obj {
				stack[n-1].wantKey = true
			} else {
				stack[n-1].idx++
			}
		}
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// safeKey shows an unknown key in a path without passing on arbitrary text.
func safeKey(k string) string {
	if len(k) > 64 || strings.ContainsFunc(k, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) {
		return "?"
	}
	return k
}

// schemaKeys is every JSON key in the three input schemas.
var schemaKeys = func() map[string]bool {
	keys := map[string]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t == reflect.TypeOf(Value{}) {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			keys[t.Field(i).Tag.Get("json")] = true
			walk(t.Field(i).Type)
		}
	}
	walk(reflect.TypeOf(Plan{}))
	walk(reflect.TypeOf(Snapshot{}))
	walk(reflect.TypeOf(Policy{}))
	return keys
}()

var errBadValue = errors.New("bad value")

// Value is a goal's from or to: an integer, a boolean or a string. Which
// one a field needs is checked by validation; null and fractions are errors.
type Value struct {
	Kind string // "int", "bool", "string", or "" when absent
	Int  int64
	Bool bool
	Str  string
}

func (v *Value) UnmarshalJSON(b []byte) error {
	switch s := string(b); {
	case s == "true" || s == "false":
		*v = Value{Kind: "bool", Bool: s == "true"}
	case strings.HasPrefix(s, `"`):
		*v = Value{Kind: "string"}
		return json.Unmarshal(b, &v.Str)
	default:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return errBadValue
		}
		*v = Value{Kind: "int", Int: n}
	}
	return nil
}

func (v Value) MarshalJSON() ([]byte, error) {
	switch v.Kind {
	case "int":
		return json.Marshal(v.Int)
	case "bool":
		return json.Marshal(v.Bool)
	}
	return json.Marshal(v.Str)
}
