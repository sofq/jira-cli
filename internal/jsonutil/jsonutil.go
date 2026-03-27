package jsonutil

import (
	"bytes"
	"encoding/json"
)

// MarshalNoEscape serializes v to JSON without HTML escaping of &, <, >.
// Returns the JSON bytes with no trailing newline.
func MarshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
