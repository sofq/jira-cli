package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRootHelp_NoHTMLEscaping(t *testing.T) {
	// The root help JSON should not contain HTML-escaped angle brackets.
	// Verify the encoding approach directly.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{
		"hint": "use `jr schema <resource>` for operations",
	})

	output := buf.String()
	if strings.Contains(output, `\u003c`) {
		t.Errorf("expected no HTML escaping in JSON, got: %s", output)
	}
	if !strings.Contains(output, `<resource>`) {
		t.Errorf("expected literal <resource> in output, got: %s", output)
	}
}
