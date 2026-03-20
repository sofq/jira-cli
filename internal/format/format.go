// Package format provides helpers to render JSON data as human-readable
// table or CSV output. Both functions accept a JSON document produced by
// WriteOutput (after any jq filtering) and return formatted bytes.
//
// Design contract:
//   - Input is expected to be a JSON array of objects for CSV and, for
//     best results, for Table as well.
//   - Column headers are sorted alphabetically for deterministic output.
//   - Nested values (objects/arrays) are stringified with json.Marshal.
//   - Table handles non-array JSON gracefully by returning the input as-is.
//   - CSV returns an error for non-array JSON.
package format

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Table converts a JSON array of objects to an aligned, human-readable text
// table written to a byte slice. Column headers are derived from the union of
// all keys across every object, sorted alphabetically.
//
// If the input is not a JSON array, the data is returned unchanged so callers
// can still display it.
func Table(data []byte) ([]byte, error) {
	// Try to parse as an array of raw messages first.
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		// Not an array — return as-is (graceful degradation).
		return data, nil
	}

	if len(rows) == 0 {
		return []byte{}, nil
	}

	// Collect all keys across rows (union), maintain insertion order first,
	// then sort for determinism.
	keySet := make(map[string]struct{})
	var keyOrder []string
	for _, raw := range rows {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			// Row is not an object — skip key collection for this row.
			continue
		}
		for k := range obj {
			if _, seen := keySet[k]; !seen {
				keySet[k] = struct{}{}
				keyOrder = append(keyOrder, k)
			}
		}
	}
	sort.Strings(keyOrder)

	// Build string rows: one []string per data row.
	type strRow = []string
	parsed := make([]strRow, 0, len(rows))
	for _, raw := range rows {
		var obj map[string]json.RawMessage
		row := make([]string, len(keyOrder))
		if err := json.Unmarshal(raw, &obj); err == nil {
			for i, k := range keyOrder {
				v, ok := obj[k]
				if !ok {
					row[i] = ""
					continue
				}
				row[i] = stringify(v)
			}
		}
		parsed = append(parsed, row)
	}

	// Compute column widths (header width vs max data width).
	widths := make([]int, len(keyOrder))
	for i, h := range keyOrder {
		widths[i] = len(h)
	}
	for _, row := range parsed {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Render header + separator + data rows.
	var buf bytes.Buffer

	writeRow := func(cells []string) {
		for i, cell := range cells {
			if i > 0 {
				buf.WriteString("  ")
			}
			buf.WriteString(padRight(cell, widths[i]))
		}
		buf.WriteByte('\n')
	}

	writeRow(keyOrder)

	// Separator line.
	sep := make([]string, len(keyOrder))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	writeRow(sep)

	for _, row := range parsed {
		writeRow(row)
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// CSV converts a JSON array of objects to CSV bytes (RFC 4180). Column headers
// are sorted alphabetically. Nested values are stringified with json.Marshal.
//
// Returns an error if the input is not a JSON array.
func CSV(data []byte) ([]byte, error) {
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("format: CSV requires a JSON array, got: %w", err)
	}

	if len(rows) == 0 {
		return []byte{}, nil
	}

	// Collect all keys (union), sorted.
	keySet := make(map[string]struct{})
	var keyOrder []string
	for _, raw := range rows {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		for k := range obj {
			if _, seen := keySet[k]; !seen {
				keySet[k] = struct{}{}
				keyOrder = append(keyOrder, k)
			}
		}
	}
	sort.Strings(keyOrder)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header row.
	if err := w.Write(keyOrder); err != nil {
		return nil, fmt.Errorf("format: csv write header: %w", err)
	}

	// Data rows.
	for _, raw := range rows {
		var obj map[string]json.RawMessage
		row := make([]string, len(keyOrder))
		if err := json.Unmarshal(raw, &obj); err == nil {
			for i, k := range keyOrder {
				v, ok := obj[k]
				if !ok {
					row[i] = ""
					continue
				}
				row[i] = stringify(v)
			}
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("format: csv write row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("format: csv flush: %w", err)
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// stringify converts a json.RawMessage to a plain string for display.
// JSON strings are unquoted. Numbers, booleans, and null are rendered as their
// JSON text representation. Objects and arrays are compact JSON.
func stringify(raw json.RawMessage) string {
	// Try to unquote a JSON string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// For numbers, booleans, null, objects, arrays — use the raw JSON.
	return string(raw)
}

// padRight pads s with spaces on the right to at least width runes.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
