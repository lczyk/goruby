// Package dumpfmt centralises the YAML / JSON output formats shared by
// the goruby debug-dump commands (parse-dump, lex-dump, parse-roundtrip).
//
// Two output shapes:
//
//   - Encode: a single value as one indented JSON document or one YAML
//     document. Used by parse-dump (whole-program AST) and
//     parse-roundtrip (single report struct).
//   - EncodeStream: a sequence of values. JSON variant emits JSONL
//     (one JSON object per line). YAML variant emits a single block
//     sequence. Used by lex-dump (one entry per token).
//
// No third-party YAML dependency: the package implements a minimal
// block-style emitter sufficient for the trees the debug-dump
// commands produce (scalars, maps, sequences, no anchors / flow / tags).
package dumpfmt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Format identifies an output format.
type Format string

const (
	FormatYAML Format = "yaml"
	FormatJSON Format = "json"
)

// ParseFormat resolves the user-facing flag string. Empty defaults to
// YAML to match the debug-dump cmds.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "", "yaml":
		return FormatYAML, nil
	case "json":
		return FormatJSON, nil
	}
	return "", fmt.Errorf("unknown format %q (want yaml or json)", s)
}

// Encode writes v as a single YAML or JSON document. JSON output is
// pretty-printed with two-space indentation.
func Encode(w io.Writer, format Format, v any) error {
	switch format {
	case FormatJSON:
		bw := bufio.NewWriter(w)
		if err := writeJSON(bw, v, 0); err != nil {
			return err
		}
		bw.WriteByte('\n')
		return bw.Flush()
	case FormatYAML:
		bw := bufio.NewWriter(w)
		if err := writeYAML(bw, v, 0, false); err != nil {
			return err
		}
		return bw.Flush()
	}
	return fmt.Errorf("dumpfmt: unsupported format %q", format)
}

// EncodeStream writes items one at a time. For JSON it emits JSONL
// (one compact JSON object per line, terminated by '\n'); for YAML it
// emits a single top-level block sequence (one `- ...` block per item).
func EncodeStream(w io.Writer, format Format, items []any) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	switch format {
	case FormatJSON:
		for _, item := range items {
			if err := writeJSONCompact(bw, item); err != nil {
				return err
			}
			bw.WriteByte('\n')
		}
		return nil
	case FormatYAML:
		for _, item := range items {
			bw.WriteString("- ")
			if err := writeYAMLSeqItem(bw, item, 0); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("dumpfmt: unsupported format %q", format)
}

// writeYAML emits v at the given indent level. inSeq is true when the
// value being emitted is the item directly after a `- ` marker -- in
// that case a map's first key sits on the same line as the marker.
func writeYAML(w *bufio.Writer, v any, indent int, inSeq bool) error {
	v = normalise(v)

	switch val := v.(type) {
	case nil:
		w.WriteString("null\n")
		return nil
	case string:
		w.WriteString(yamlScalarString(val))
		w.WriteByte('\n')
		return nil
	case bool:
		w.WriteString(strconv.FormatBool(val))
		w.WriteByte('\n')
		return nil
	case int64:
		w.WriteString(strconv.FormatInt(val, 10))
		w.WriteByte('\n')
		return nil
	case uint64:
		w.WriteString(strconv.FormatUint(val, 10))
		w.WriteByte('\n')
		return nil
	case float64:
		w.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		w.WriteByte('\n')
		return nil
	case []any:
		if len(val) == 0 {
			w.WriteString("[]\n")
			return nil
		}
		// Sequence starts on the next line at the same indent as the
		// parent key (no extra indent for `-`), unless we're directly
		// under a key in which case the caller already emitted `\n`.
		for _, item := range val {
			writeIndent(w, indent)
			w.WriteString("- ")
			if err := writeYAMLSeqItem(w, item, indent); err != nil {
				return err
			}
		}
		_ = inSeq
		return nil
	case map[string]any:
		return writeYAMLMap(w, val, indent, inSeq)
	}
	return fmt.Errorf("dumpfmt: unsupported value type %T", v)
}

// writeYAMLSeqItem writes one element of a sequence, given that the
// caller has already emitted the leading `- ` marker.
func writeYAMLSeqItem(w *bufio.Writer, item any, indent int) error {
	item = normalise(item)
	if m, ok := item.(map[string]any); ok && len(m) > 0 {
		// First key sits on the same line as the `- `; subsequent
		// keys indent two further than the marker.
		return writeYAMLMap(w, m, indent+1, true)
	}
	// Scalar / sequence / empty map: emit inline after `- `.
	switch v := item.(type) {
	case []any:
		if len(v) == 0 {
			w.WriteString("[]\n")
			return nil
		}
		w.WriteByte('\n')
		return writeYAML(w, v, indent+1, false)
	case map[string]any:
		w.WriteString("{}\n")
		return nil
	}
	return writeYAML(w, item, indent+1, false)
}

func writeYAMLMap(w *bufio.Writer, m map[string]any, indent int, inSeq bool) error {
	if len(m) == 0 {
		w.WriteString("{}\n")
		return nil
	}
	keys := mapKeys(m)
	for i, k := range keys {
		if inSeq && i == 0 {
			// Already on the line after `- ` -- do not re-indent.
		} else {
			writeIndent(w, indent)
		}
		w.WriteString(k)
		w.WriteString(":")
		v := normalise(m[k])
		if isInlineScalar(v) {
			w.WriteByte(' ')
			if err := writeYAML(w, v, indent+1, false); err != nil {
				return err
			}
			continue
		}
		// Non-scalar values land on the next line, indented one level.
		w.WriteByte('\n')
		if err := writeYAML(w, v, indent+1, false); err != nil {
			return err
		}
	}
	return nil
}

// writeJSON emits v as a pretty-printed JSON value with two-space
// indentation. Map keys are sorted alphabetically to give stable output.
func writeJSON(w *bufio.Writer, v any, indent int) error {
	v = normalise(v)
	switch val := v.(type) {
	case nil:
		w.WriteString("null")
		return nil
	case bool:
		if val {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
		return nil
	case int64:
		w.WriteString(strconv.FormatInt(val, 10))
		return nil
	case uint64:
		w.WriteString(strconv.FormatUint(val, 10))
		return nil
	case float64:
		w.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		return nil
	case string:
		writeJSONString(w, val)
		return nil
	case []any:
		if len(val) == 0 {
			w.WriteString("[]")
			return nil
		}
		w.WriteString("[\n")
		for i, item := range val {
			writeIndent(w, indent+1)
			if err := writeJSON(w, item, indent+1); err != nil {
				return err
			}
			if i < len(val)-1 {
				w.WriteByte(',')
			}
			w.WriteByte('\n')
		}
		writeIndent(w, indent)
		w.WriteByte(']')
		return nil
	case map[string]any:
		if len(val) == 0 {
			w.WriteString("{}")
			return nil
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteString("{\n")
		for i, k := range keys {
			writeIndent(w, indent+1)
			writeJSONString(w, k)
			w.WriteString(": ")
			if err := writeJSON(w, val[k], indent+1); err != nil {
				return err
			}
			if i < len(keys)-1 {
				w.WriteByte(',')
			}
			w.WriteByte('\n')
		}
		writeIndent(w, indent)
		w.WriteByte('}')
		return nil
	}
	return fmt.Errorf("dumpfmt: unsupported value type %T", v)
}

// writeJSONCompact emits v as a single-line JSON value (no whitespace).
// Used for JSONL streaming.
func writeJSONCompact(w *bufio.Writer, v any) error {
	v = normalise(v)
	switch val := v.(type) {
	case nil:
		w.WriteString("null")
		return nil
	case bool:
		if val {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
		return nil
	case int64:
		w.WriteString(strconv.FormatInt(val, 10))
		return nil
	case uint64:
		w.WriteString(strconv.FormatUint(val, 10))
		return nil
	case float64:
		w.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		return nil
	case string:
		writeJSONString(w, val)
		return nil
	case []any:
		w.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				w.WriteByte(',')
			}
			if err := writeJSONCompact(w, item); err != nil {
				return err
			}
		}
		w.WriteByte(']')
		return nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				w.WriteByte(',')
			}
			writeJSONString(w, k)
			w.WriteByte(':')
			if err := writeJSONCompact(w, val[k]); err != nil {
				return err
			}
		}
		w.WriteByte('}')
		return nil
	}
	return fmt.Errorf("dumpfmt: unsupported value type %T", v)
}

// writeJSONString writes s as a JSON-quoted string. Handles the
// mandatory escapes (\", \\, control chars) and leaves the rest as-is.
func writeJSONString(w *bufio.Writer, s string) {
	w.WriteByte('"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 {
			if start < i {
				w.WriteString(s[start:i])
			}
			switch c {
			case '"':
				w.WriteString(`\"`)
			case '\\':
				w.WriteString(`\\`)
			case '\n':
				w.WriteString(`\n`)
			case '\r':
				w.WriteString(`\r`)
			case '\t':
				w.WriteString(`\t`)
			case '\b':
				w.WriteString(`\b`)
			case '\f':
				w.WriteString(`\f`)
			default:
				w.WriteString(`\u00`)
				const hex = "0123456789abcdef"
				w.WriteByte(hex[c>>4])
				w.WriteByte(hex[c&0xF])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		w.WriteString(s[start:])
	}
	w.WriteByte('"')
}

func writeIndent(w *bufio.Writer, level int) {
	for range level {
		w.WriteString("  ")
	}
}

// isInlineScalar reports whether v fits on the same line as its key
// (i.e. is a scalar or a marker for empty collection).
func isInlineScalar(v any) bool {
	switch x := v.(type) {
	case nil, string, bool, int64, uint64, float64:
		return true
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// mapKeys returns m's keys sorted with `_type` first (when present) and
// the rest alphabetically. Putting the type tag at the top of a node's
// dump makes the polymorphic tree easier to scan.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Move "_type" to the front if present (sorted position varies).
	for i, k := range keys {
		if k == "_type" {
			if i != 0 {
				copy(keys[1:i+1], keys[:i])
				keys[0] = "_type"
			}
			break
		}
	}
	return keys
}

// yamlScalarString formats a string as a YAML scalar. Anything with
// special characters, leading/trailing whitespace, or shapes that could
// be mistaken for a YAML keyword gets JSON-quoted (which is a strict
// subset of YAML double-quoted strings).
func yamlScalarString(s string) string {
	if s == "" {
		return `""`
	}
	if needsYAMLQuoting(s) {
		b, _ := json.Marshal(s)
		return string(b)
	}
	return s
}

func needsYAMLQuoting(s string) bool {
	if strings.TrimSpace(s) != s {
		return true
	}
	switch s {
	case "null", "true", "false", "yes", "no", "~":
		return true
	}
	// If it parses cleanly as a number, quote it to keep string shape.
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	for i, r := range s {
		switch r {
		case '\n', '\r', '\t', '\\', '"', '\'':
			return true
		case ':', '#', '{', '}', '[', ']', ',', '&', '*', '!', '|', '>', '%', '@', '`':
			return true
		case '-', '?':
			if i == 0 && (len(s) == 1 || s[1] == ' ') {
				return true
			}
		}
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// TypedTree converts v into a nested map / slice / scalar tree where
// each struct becomes a map carrying a `_type` key naming its concrete
// type (e.g. `*ast.Identifier`). Zero-valued exported fields are
// omitted to keep debug-dump output compact. Useful for polymorphic
// trees where the consumer needs to tell concrete types behind
// interface fields apart.
func TypedTree(v any) any {
	return typedTree(reflect.ValueOf(v), "")
}

func typedTree(rv reflect.Value, typeName string) any {
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return typedTree(rv.Elem(), "*"+rv.Type().Elem().String())
	case reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return typedTree(rv.Elem(), "")
	case reflect.Struct:
		out := map[string]any{}
		if typeName == "" {
			typeName = rv.Type().String()
		}
		out["_type"] = typeName
		t := rv.Type()
		for i := range rv.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fv := rv.Field(i)
			if fv.IsZero() {
				continue
			}
			out[f.Name] = typedTree(fv, "")
		}
		return out
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			return []any{}
		}
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = typedTree(rv.Index(i), "")
		}
		return out
	case reflect.Map:
		out := map[string]any{}
		iter := rv.MapRange()
		for iter.Next() {
			k := fmt.Sprint(iter.Key().Interface())
			out[k] = typedTree(iter.Value(), "")
		}
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint()
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	}
	return fmt.Sprint(rv.Interface())
}

// normalise widens go's numeric types to the int64/uint64/float64 set
// the emitter handles, and replaces reflect-zero values with nil so the
// rest of the emitter can stay type-switch driven.
func normalise(v any) any {
	// Fast path: canonical types pass through without touching reflect.
	switch x := v.(type) {
	case nil:
		return nil
	case string, bool, int64, uint64, float64, []any, map[string]any:
		return v
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case uint:
		return uint64(x)
	case uint8:
		return uint64(x)
	case uint16:
		return uint64(x)
	case uint32:
		return uint64(x)
	case uintptr:
		return uint64(x)
	case float32:
		return float64(x)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint()
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = rv.Index(i).Interface()
		}
		return out
	case reflect.Map:
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = iter.Value().Interface()
		}
		return out
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return normalise(rv.Elem().Interface())
	case reflect.Struct:
		out := make(map[string]any, rv.NumField())
		t := rv.Type()
		for i := range rv.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			out[f.Name] = rv.Field(i).Interface()
		}
		return out
	}
	return v
}
