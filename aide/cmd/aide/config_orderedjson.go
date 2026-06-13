package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// orderedMap is a JSON object that remembers its key insertion order. The
// `aide config` writer uses it so that editing one key patches the file in
// place instead of re-emitting every key in sorted order — important because a
// user's aide.json (especially the global ~/.aide one) may be co-managed by
// other tools, and we must not churn keys we didn't touch.
//
// Decoded nested objects are themselves *orderedMap; arrays stay []any and
// scalars stay as the stdlib decodes them (string, bool, nil, json.Number).
type orderedMap struct {
	keys []string
	vals map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{vals: map[string]any{}}
}

func (o *orderedMap) get(k string) (any, bool) {
	v, ok := o.vals[k]
	return v, ok
}

// set stores v, appending the key on first insert and preserving its position
// on update.
func (o *orderedMap) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedMap) delete(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *orderedMap) len() int { return len(o.keys) }

// decodeOrderedJSON parses JSON, decoding every object as an *orderedMap so key
// order survives a round-trip. Uses json.Number so integers don't widen to
// float and re-render differently.
func decodeOrderedJSON(data []byte) (*orderedMap, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, err
	}
	om, ok := v.(*orderedMap)
	if !ok {
		return nil, fmt.Errorf("config root must be a JSON object")
	}
	return om, nil
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // scalar: string, bool, nil, json.Number
	}
	switch delim {
	case '{':
		om := newOrderedMap()
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			om.set(keyTok.(string), val)
		}
		if _, err := dec.Token(); err != nil { // consume '}'
			return nil, err
		}
		return om, nil
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil { // consume ']'
			return nil, err
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unexpected delimiter %v", delim)
	}
}

// encodeOrderedJSON renders v as 2-space-indented JSON, emitting object keys in
// their stored order. The byte layout matches json.MarshalIndent (separators,
// expanded arrays, HTML-escaped scalars) so only the edited key differs from
// what the previous sorted writer produced.
func encodeOrderedJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeOrderedValue(&buf, v, ""); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeOrderedValue(buf *bytes.Buffer, v any, indent string) error {
	switch t := v.(type) {
	case *orderedMap:
		if t.len() == 0 {
			buf.WriteString("{}")
			return nil
		}
		inner := indent + "  "
		buf.WriteString("{\n")
		for i, k := range t.keys {
			buf.WriteString(inner)
			if err := encodeScalar(buf, k); err != nil {
				return err
			}
			buf.WriteString(": ")
			if err := encodeOrderedValue(buf, t.vals[k], inner); err != nil {
				return err
			}
			if i < len(t.keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent + "}")
		return nil
	case []any:
		return encodeArray(buf, len(t), func(i int) any { return t[i] }, indent)
	case []string:
		return encodeArray(buf, len(t), func(i int) any { return t[i] }, indent)
	default:
		return encodeScalar(buf, v)
	}
}

func encodeArray(buf *bytes.Buffer, n int, at func(int) any, indent string) error {
	if n == 0 {
		buf.WriteString("[]")
		return nil
	}
	inner := indent + "  "
	buf.WriteString("[\n")
	for i := 0; i < n; i++ {
		buf.WriteString(inner)
		if err := encodeOrderedValue(buf, at(i), inner); err != nil {
			return err
		}
		if i < n-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(indent + "]")
	return nil
}

// encodeScalar marshals a leaf (or an object key) exactly as encoding/json
// would, including HTML escaping, so output stays byte-compatible with the
// previous json.MarshalIndent writer.
func encodeScalar(buf *bytes.Buffer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	buf.Write(b)
	return nil
}
