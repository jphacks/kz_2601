package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// object is a JSON object that keeps the key order of the schema, so the
// generated Kotlin follows the schema order and diffs stay readable.
type object struct {
	keys []string
	vals map[string]any
}

// Accessors are nil-safe so lookups can be chained.
func (o *object) has(k string) bool {
	if o == nil {
		return false
	}
	_, ok := o.vals[k]
	return ok
}

func (o *object) get(k string) any {
	if o == nil {
		return nil
	}
	return o.vals[k]
}

func (o *object) str(k string) string {
	s, _ := o.get(k).(string)
	return s
}

func (o *object) obj(k string) *object {
	v, _ := o.get(k).(*object)
	return v
}

func (o *object) arr(k string) []any {
	v, _ := o.get(k).([]any)
	return v
}

// strings returns the string items of an array value (enum, required, ...).
func (o *object) strings(k string) []string {
	var out []string
	for _, v := range o.arr(k) {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func parseOrdered(data []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	o, ok := v.(*object)
	if !ok {
		return nil, fmt.Errorf("schema root is not an object")
	}
	return o, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch d {
	case '{':
		o := &object{vals: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, _ := kt.(string)
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			o.keys = append(o.keys, k)
			o.vals[k] = v
		}
		_, err := dec.Token()
		return o, err
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token()
		return arr, err
	}
	return nil, fmt.Errorf("unexpected delimiter %v", d)
}
