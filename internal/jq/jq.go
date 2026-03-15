package jq

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// Apply runs a jq filter expression on JSON input and returns the result as JSON bytes.
// If filter is empty, returns input unchanged.
func Apply(input []byte, filter string) ([]byte, error) {
	if filter == "" {
		return input, nil
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid jq filter: %w", err)
	}

	var data interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON input: %w", err)
	}

	iter := query.Run(data)
	var results []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq error: %w", err)
		}
		results = append(results, v)
	}

	if len(results) == 0 {
		return []byte("null"), nil
	}

	if len(results) == 1 {
		result, err := json.Marshal(results[0])
		if err != nil {
			return nil, fmt.Errorf("marshaling jq result: %w", err)
		}
		return result, nil
	}

	result, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("marshaling jq results: %w", err)
	}
	return result, nil
}
