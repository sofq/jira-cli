package jq_test

import (
	"testing"

	"github.com/sofq/jira-cli/internal/jq"
)

func FuzzApply(f *testing.F) {
	// Seed corpus with representative inputs.
	f.Add([]byte(`{"key":"PROJ-1"}`), ".")
	f.Add([]byte(`{"a":1,"b":2}`), ".a + .b")
	f.Add([]byte(`[1,2,3]`), "[.[] | . * 2]")
	f.Add([]byte(`{"x":"hello"}`), ".x | length")
	f.Add([]byte(`null`), ".")
	f.Add([]byte(`{"items":[{"n":1},{"n":2}]}`), "[.items[].n]")

	f.Fuzz(func(t *testing.T, input []byte, filter string) {
		// Apply must not panic on any input. Errors are fine.
		jq.Apply(input, filter) //nolint:errcheck
	})
}
